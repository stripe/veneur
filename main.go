package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	statsd "github.com/DataDog/datadog-go/statsd"
	log "github.com/Sirupsen/logrus"
)

var (
	apiURL     = flag.String("apiurl", "https://app.datadoghq.com", "The URL to which Metrics will be posted")
	bufferSize = flag.Int("buffersize", 4096, "The size of the buffer of work for the worker pool")
	debug      = flag.Bool("d", false, "Enable debug logging")
	expiry     = flag.Duration("expiry", 5*time.Minute, "The duration metrics will be retained if they stop showing up, see go's ParseDuration")
	interval   = flag.Duration("i", 10*time.Second, "The interval at which to flush metrics, see go's ParseDuration")
	key        = flag.String("key", "fart", "Your Datadog API Key")
	udpAddr    = flag.String("listen", ":8126", "Address to listen for UDP requests on")
	nWorkers   = flag.Int("n", 4, "The number of workers to start")
	sampleRate = flag.Float64("sample", 0.01, "The sample rate for packet receive counts")
	statsAddr  = flag.String("stats", "localhost:8125", "Address of DogStatsD instance to send internal metrics")
)

func main() {
	// Parse the command-line flags.
	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	stats, err := statsd.New(*statsAddr)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error creating statsd logging")
	}
	stats.Namespace = "veneur."

	// Start the dispatcher.
	log.WithFields(log.Fields{
		"number": *nWorkers,
	}).Info("Starting workers")
	workers := make([]*Worker, *nWorkers)
	for i := 0; i < *nWorkers; i++ {
		worker := NewWorker(i + 1)
		worker.Start()
		workers[i] = worker
	}

	// Start the UDP server!
	log.WithFields(log.Fields{
		"address":        *udpAddr,
		"flush_interval": *interval,
		"expiry":         *expiry,
	}).Info("UDP server listening")
	serverAddr, err := net.ResolveUDPAddr("udp", *udpAddr)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Error resolving address")
	}

	ServerConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Error listening for UDP")
	}

	defer ServerConn.Close()

	ticker := time.NewTicker(*interval)
	go func() {
		for t := range ticker.C {
			metrics := make([][]DDMetric, *nWorkers)
			flushTime := time.Now()
			for i, w := range workers {
				log.WithFields(log.Fields{
					"worker": i,
					"tick":   t,
				}).Debug("Flushing")
				wstart := time.Now()
				metrics = append(metrics, w.Flush(*interval, *expiry, flushTime))
				// Track how much time each worker takes to flush.
				stats.TimeInMilliseconds(
					"flush_worker_duration_ms",
					float64(time.Now().Sub(wstart).Nanoseconds()),
					[]string{fmt.Sprintf("worker:%d", i)},
					1.0,
				)
			}
			fstart := time.Now()
			flush(metrics, stats)
			stats.TimeInMilliseconds(
				"flush_transaction_duration_ms",
				float64(time.Now().Sub(fstart).Nanoseconds()),
				nil,
				1.0,
			)
		}
	}()

	buf := make([]byte, *bufferSize)

	for {
		n, _, err := ServerConn.ReadFromUDP(buf)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error reading from UDP")
		}

		// We could maybe free up the Read above by moving
		// this part to a buffered channel?
		m, err := ParseMetric(buf[:n])
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error parsing packet")
			stats.Count("packet.error_total", 1, nil, 1.0)
			continue
		}
		stats.Count(
			"packet.received_total",
			1,
			[]string{fmt.Sprintf("type:%s", m.Type)},
			*sampleRate,
		)

		// Hash the incoming key so we can consistently choose a worker
		// by modding the last byte
		h := fnv.New32()
		h.Write([]byte(m.Name))
		index := h.Sum32() % uint32(*nWorkers)

		// We're ready to have a worker process this packet, so add it
		// to the work queue. Note that if the queue is full, we'll block
		// here.
		workers[index].WorkChan <- *m
	}
}

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func flush(postMetrics [][]DDMetric, stats *statsd.Client) {
	totalCount := 0
	var finalMetrics []DDMetric
	// TODO This seems very inefficient
	for _, metrics := range postMetrics {
		totalCount += len(metrics)
		finalMetrics = append(finalMetrics, metrics...)
	}
	// Check to see if we have anything to do
	if totalCount > 0 {
		stats.Count("metrics_flushed_total", int64(totalCount), nil, 1.0)
		// TODO Watch this error
		postJSON, _ := json.Marshal(map[string][]DDMetric{
			"series": finalMetrics,
		})

		resp, err := http.Post(fmt.Sprintf("%s/api/v1/series?api_key=%s", *apiURL, *key), "application/json", bytes.NewBuffer(postJSON))
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error posting")
		} else {
			log.WithFields(log.Fields{
				"metrics": len(finalMetrics),
			}).Info("Completed flush to Datadog")
		}
		if log.GetLevel() == log.DebugLevel {
			defer resp.Body.Close()
			// TODO Watch this error
			body, _ := ioutil.ReadAll(resp.Body)
			log.WithFields(log.Fields{
				"json":     string(postJSON),
				"status":   resp.Status,
				"headers":  resp.Header,
				"response": body,
			}).Debug("POSTing JSON")
		}
	} else {
		log.Info("Nothing to flush, skipping.")
	}
}
