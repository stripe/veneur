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
	"github.com/gphat/veneur"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func main() {
	flag.Parse()

	if configFile == nil || *configFile == "" {
		log.Fatal("You must specify a config file")
	}

	err := veneur.ReadConfig(*configFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error reading config file")
	}

	// Parse the command-line flags.
	flag.Parse()

	if veneur.Config.Debug {
		log.SetLevel(log.DebugLevel)
		log.WithFields(log.Fields{
			"config": veneur.Config.Debug,
		}).Debug("Starting with config")
	}

	veneur.InitStats()

	// Start the dispatcher.
	log.WithFields(log.Fields{
		"number": veneur.Config.NumWorkers,
	}).Info("Starting workers")
	workers := make([]*veneur.Worker, veneur.Config.NumWorkers)
	for i := 0; i < veneur.Config.NumWorkers; i++ {
		worker := veneur.NewWorker(i + 1)
		worker.Start()
		workers[i] = worker
	}

	// Start the UDP server!
	log.WithFields(log.Fields{
		"address":        veneur.Config.UDPAddr,
		"flush_interval": veneur.Config.Interval,
	}).Info("UDP server listening")
	serverAddr, err := net.ResolveUDPAddr("udp", veneur.Config.UDPAddr)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Error resolving address")
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)

	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Error listening for UDP")
	}
	serverConn.SetReadBuffer(veneur.Config.ReadBufferSizeBytes) // TODO Configurable!

	defer serverConn.Close()

	ticker := time.NewTicker(veneur.Config.Interval)
	go func() {
		for t := range ticker.C {
			metrics := make([][]veneur.DDMetric, veneur.Config.NumWorkers)
			for i, w := range workers {
				log.WithFields(log.Fields{
					"worker": i,
					"tick":   t,
				}).Debug("Flushing")
				metrics = append(metrics, w.Flush())
			}
			fstart := time.Now()
			flush(metrics)
			veneur.Stats.TimeInMilliseconds(
				"flush.transaction_duration_ns",
				float64(time.Now().Sub(fstart).Nanoseconds()),
				nil,
				1.0,
			)
		}
	}()

	// Creates N workers to handle incoming packets, parsing them,
	// hashing them and dispatching them on to workers that do the storage.
	parserChan := make(chan []byte)
	for i := 0; i < veneur.Config.NumWorkers; i++ {
		go func() {
			for {
				select {
				case m := <-parserChan:
					handlePacket(workers, m)
				}
			}
		}()
	}

	// Read forever!
	for {
		buf := make([]byte, veneur.Config.MetricMaxLength)
		n, _, err := serverConn.ReadFromUDP(buf)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error reading from UDP")
			continue
		}
		select {
		case parserChan <- buf[:n]:
		}
	}
}

func handlePacket(workers []*veneur.Worker, packet []byte) {
	m, err := veneur.ParseMetric(packet)
	if err != nil {
		log.WithFields(log.Fields{
			"error":  err,
			"packet": packet,
		}).Error("Error parsing packet")
		veneur.Stats.Count("packet.error_total", 1, nil, 1.0)
		return
	}

	// Hash the incoming key so we can consistently choose a worker
	// by modding the digest
	h := fnv.New32()
	h.Write([]byte(m.Name))
	index := h.Sum32() % uint32(veneur.Config.NumWorkers)

	// We're ready to have a worker process this packet, so add it
	// to the work queue.
	select {
	case workers[index].WorkChan <- *m:
	}
}

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func flush(postMetrics [][]veneur.DDMetric) {
	totalCount := 0
	var finalMetrics []veneur.DDMetric
	// TODO This seems very inefficient
	for _, metrics := range postMetrics {
		totalCount += len(metrics)
		finalMetrics = append(finalMetrics, metrics...)
	}
	// Check to see if we have anything to do
	if totalCount > 0 {
		veneur.Stats.Count("flush.metrics_total", int64(totalCount), nil, 1.0)
		// TODO Watch this error
		postJSON, _ := json.Marshal(map[string][]veneur.DDMetric{
			"series": finalMetrics,
		})

		resp, err := http.Post(fmt.Sprintf("%s/api/v1/series?api_key=%s", veneur.Config.APIHostname, veneur.Config.Key), "application/json", bytes.NewBuffer(postJSON))
		if err != nil {
			veneur.Stats.Count("flush.error_total", int64(totalCount), nil, 1.0)
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
