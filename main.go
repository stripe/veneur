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

	log "github.com/Sirupsen/logrus"
)

var (
	debug         = flag.Bool("d", false, "Enable debug logging")
	nWorkers      = flag.Int("n", 4, "The number of workers to start")
	interval      = flag.Duration("i", 10*time.Second, "The interval at which to flush metrics, see go's ParseDuration")
	udpAddr       = flag.String("http", ":8125", "Address to listen for UDP requests on")
	key           = flag.String("key", "fart", "Your Datadog API Key")
	expirySeconds = flag.Duration("expiry", 5*time.Minute, "The duration metrics will be retained if they stop showing up, see go's ParseDuration")
	bufferSize    = flag.Int("buffersize", 4096, "The size of the buffer of work for the worker pool")
	apiURL        = flag.String("apiurl", "https://app.datadoghq.com", "The URL to which Metrics will be posted")
)

/* The WorkQueue is a buffered channel that we can send work to */
// TODO This should likely be configurable or unbuffered?
// var WorkQueue = make(chan PacketRequest, 100)

func main() {
	// Parse the command-line flags.
	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

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
		"expiry":         *expirySeconds,
	}).Info("UDP server listening")
	serverAddr, err := net.ResolveUDPAddr("udp", ":8125")
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
			for i, w := range workers {
				log.WithFields(log.Fields{
					"worker": i,
					"tick":   t,
				}).Debug("Flushing")
				metrics = append(metrics, w.Flush(*interval, *expirySeconds, time.Now()))
			}
			flush(metrics)
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
			log.Error("Error parsing packet: ", err)
			// TODO A metric!
			continue
		}

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
func flush(postMetrics [][]DDMetric) {
	totalCount := 0
	var finalMetrics []DDMetric
	// TODO This seems very inefficient
	for _, metrics := range postMetrics {
		totalCount += len(metrics)
		finalMetrics = append(finalMetrics, metrics...)
	}
	// Check to see if we have anything to do
	if totalCount > 0 {
		// Make a metric for how many metrics we metriced (be sure to add one to the total for it!)
		// finalMetrics = append(
		// 	finalMetrics,
		// 	// TODO Is this the right type?
		// 	NewPostMetric("veneur.stats.metrics_posted", float32(totalCount+1), "", "counter", *Interval),
		// )
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
