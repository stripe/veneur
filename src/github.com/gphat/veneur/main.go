package main

import (
	"flag"
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
var WorkQueue = make(chan PacketRequest, 100)

func main() {
	// Parse the command-line flags.
	flag.Parse()

	// Start the dispatcher.
	log.Println("Starting the dispatcher")
	StartDispatcher(*NWorkers, WorkQueue)

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

	// TODO More than 1024!!
	buf := make([]byte, 1024)

	for {
		n, _, err := ServerConn.ReadFromUDP(buf)

		work := PacketRequest{Packet: string(buf[:n])}

		// We're ready to have a worker process this packet, so add it
		// to the work queue. Note that if the queue is full, we'll block
		// here.
		WorkQueue <- work

		if err != nil {
			log.Println("Error: ", err)
		}
	}
}
