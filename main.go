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
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func main() {

	flag.Parse()

	if configFile == nil || *configFile == "" {
		log.Fatal("You must specify a config file")
	}

	config, err := ReadConfig(*configFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error reading config file")
	}

	// Parse the command-line flags.
	flag.Parse()

	if config.Debug {
		log.SetLevel(log.DebugLevel)
		log.WithFields(log.Fields{
			"config": config,
		}).Debug("Starting with config")
	}

	stats, err := statsd.New(config.StatsAddr)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error creating statsd logging")
	}
	stats.Namespace = "veneur."

	// Start the dispatcher.
	log.WithFields(log.Fields{
		"number": config.NumWorkers,
	}).Info("Starting workers")
	workers := make([]*Worker, config.NumWorkers)
	for i := 0; i < config.NumWorkers; i++ {
		worker := NewWorker(i+1, config.Percentiles)
		worker.Start()
		workers[i] = worker
	}

	// Start the UDP server!
	log.WithFields(log.Fields{
		"address":        config.UDPAddr,
		"flush_interval": config.Interval,
		"expiry":         config.Expiry,
	}).Info("UDP server listening")
	serverAddr, err := net.ResolveUDPAddr("udp", config.UDPAddr)
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

	ticker := time.NewTicker(config.Interval)
	go func() {
		for t := range ticker.C {
			metrics := make([][]DDMetric, config.NumWorkers)
			flushTime := time.Now()
			for i, w := range workers {
				log.WithFields(log.Fields{
					"worker": i,
					"tick":   t,
				}).Debug("Flushing")
				wstart := time.Now()
				metrics = append(metrics, w.Flush(config.Interval, config.Expiry, flushTime))
				// Track how much time each worker takes to flush.
				stats.TimeInMilliseconds(
					"flush.worker_duration_ns",
					float64(time.Now().Sub(wstart).Nanoseconds()),
					[]string{fmt.Sprintf("worker:%d", i)},
					1.0,
				)
			}
			fstart := time.Now()
			flush(config, metrics, stats)
			stats.TimeInMilliseconds(
				"flush.transaction_duration_ns",
				float64(time.Now().Sub(fstart).Nanoseconds()),
				nil,
				1.0,
			)
		}
	}()

	buf := make([]byte, config.BufferSize)

	for {
		n, _, err := ServerConn.ReadFromUDP(buf)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error reading from UDP")
		}

		pstart := time.Now()
		// We could maybe free up the Read above by moving
		// this part to a buffered channel?
		m, err := ParseMetric(buf[:n], config.Tags)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error parsing packet")
			stats.Count("packet.error_total", 1, nil, 1.0)
			continue
		}
		stats.TimeInMilliseconds(
			"packet.parse_duration_ns",
			float64(time.Now().Sub(pstart).Nanoseconds()),
			[]string{fmt.Sprintf("type:%s", m.Type)},
			config.SampleRate,
		)

		// Hash the incoming key so we can consistently choose a worker
		// by modding the last byte
		h := fnv.New32()
		h.Write([]byte(m.Name))
		index := h.Sum32() % uint32(config.NumWorkers)

		// We're ready to have a worker process this packet, so add it
		// to the work queue. Note that if the queue is full, we'll block
		// here.
		workers[index].WorkChan <- *m
	}
}

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func flush(config *VeneurConfig, postMetrics [][]DDMetric, stats *statsd.Client) {
	totalCount := 0
	var finalMetrics []DDMetric
	// TODO This seems very inefficient
	for _, metrics := range postMetrics {
		totalCount += len(metrics)
		finalMetrics = append(finalMetrics, metrics...)
	}
	// Check to see if we have anything to do
	if totalCount > 0 {
		stats.Count("flush.metrics_total", int64(totalCount), nil, 1.0)
		// TODO Watch this error
		postJSON, _ := json.Marshal(map[string][]DDMetric{
			"series": finalMetrics,
		})

		resp, err := http.Post(fmt.Sprintf("%s/api/v1/series?api_key=%s", config.APIURL, config.Key), "application/json", bytes.NewBuffer(postJSON))
		if err != nil {
			stats.Count("flush.error_total", int64(totalCount), nil, 1.0)
			// TODO Do something at failure time!
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
