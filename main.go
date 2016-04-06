package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

var (
	NWorkers      = flag.Int("n", 4, "The number of workers to start")
	Interval      = flag.Int("i", 10, "The interval in seconds at which to flush metrics")
	UDPAddr       = flag.String("http", ":8125", "Address to listen for UDP requests on")
	Key           = flag.String("key", "fart", "Your Datadog API Key")
	ExpirySeconds = flag.Int("expiry", 300, "The number of seconds metrics will be retained if they stop showing up")
	BufferSize    = flag.Int("buffersize", 4096, "The size of the buffer of work for the worker pool")
)

/* The WorkQueue is a buffered channel that we can send work to */
// TODO This should likely be configurable or unbuffered?
// var WorkQueue = make(chan PacketRequest, 100)

func main() {
	// Parse the command-line flags.
	flag.Parse()

	// Start the dispatcher.
	log.Println("Starting workers")
	workers := make([]*Worker, *NWorkers)
	for i := 0; i < *NWorkers; i++ {
		worker := NewWorker(i+1, *Interval, *ExpirySeconds)
		worker.Start()
		workers[i] = worker
	}

	// Start the UDP server!
	log.Println("UDP server listening on", *UDPAddr)
	ServerAddr, err := net.ResolveUDPAddr("udp", ":8125")
	if err != nil {
		log.Println(err.Error())
	}

	ServerConn, err := net.ListenUDP("udp", ServerAddr)
	if err != nil {
		log.Println(err.Error())
	}

	defer ServerConn.Close()

	ticker := time.NewTicker(time.Duration(*Interval) * time.Second)
	go func() {
		for t := range ticker.C {
			metrics := make([][]DDMetric, *NWorkers)
			for i, w := range workers {
				log.Printf("Flushing worker %d at %v", i, t)
				metrics = append(metrics, w.Flush(time.Now()))
			}
			flush(metrics)
		}
	}()

	buf := make([]byte, *BufferSize)

	for {
		n, _, err := ServerConn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Error reading from UDP: ", err)
		}

		// We could maybe free up the Read above by moving
		// this part to a buffered channel?
		m, err := ParseMetric(buf[:n])
		if err != nil {
			log.Println("Error parsing packet: ", err)
			// TODO A metric!
			continue
		}

		// Hash the incoming key so we can consistently choose a worker
		// by modding the last byte
		h := fnv.New32()
		h.Write([]byte(m.Name))
		index := h.Sum32() % uint32(*NWorkers)

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
		postJSON, _ := json.MarshalIndent(map[string][]DDMetric{
			"series": finalMetrics,
		}, "", "  ")

		fmt.Println(string(postJSON))

		resp, err := http.Post(fmt.Sprintf("https://app.datadoghq.com/api/v1/series?api_key=%s", *Key), "application/json", bytes.NewBuffer(postJSON))
		if err != nil {
			log.Printf("Error posting: %q", err)
		}
		defer resp.Body.Close()
		fmt.Println("Response Status:", resp.Status)
		fmt.Println("Response Headers:", resp.Header)
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response body: %q", err)
		}
		fmt.Printf("Response: %q", body)
	} else {
		log.Println("Nothing to flush.")
	}
}
