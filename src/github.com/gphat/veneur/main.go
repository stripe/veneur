package main

import (
	"flag"
	"hash/fnv"
	"log"
	"net"
)

var (
	NWorkers = flag.Int("n", 4, "The number of workers to start")
	Interval = flag.Int("i", 10, "The interval at which to flush metrics")
	UDPAddr  = flag.String("http", ":8125", "Address to listen for UDP requests on")
)

/* The WorkQueue is a buffered channel that we can send work to */
// TODO This should likely be configurable or unbuffered?
// var WorkQueue = make(chan PacketRequest, 100)

func main() {
	// Parse the command-line flags.
	flag.Parse()

	// Start the dispatcher.
	log.Println("Starting workers")

	workers := make([]Worker, *NWorkers)
	for i := 0; i < *NWorkers; i++ {
		worker := NewWorker(i+1, *Interval)
		worker.Start()
		workers[i] = worker
	}

	// StartDispatcher(*NWorkers, WorkQueue)

	// Start the HTTP server!
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

	// TODO More than 4096?
	buf := make([]byte, 4096)

	for {
		n, _, err := ServerConn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Error: ", err)
		}

		// We could maybe free up the Read above by moving
		// this part to a buffered channel?
		m, err := ParseMetric(string(buf[:n]))
		if err != nil {
			log.Println("Error parsing packet: ", err)
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
