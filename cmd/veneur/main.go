package main

import (
	"flag"
	"net"
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
		log.WithError(err).Fatal("Error reading config file")
	}

	// Parse the command-line flags.
	flag.Parse()

	if veneur.Config.Debug {
		log.SetLevel(log.DebugLevel)
		log.WithField("config", veneur.Config).Debug("Starting with config")
	}

	veneur.InitStats()

	// Start the dispatcher.
	log.WithField("number", veneur.Config.NumWorkers).Info("Starting workers")
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
		log.WithError(err).Fatal("Error resolving address")
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)

	if err != nil {
		log.WithError(err).Fatal("Error listening for UDP")
	}
	if err := serverConn.SetReadBuffer(veneur.Config.ReadBufferSizeBytes); err != nil {
		log.WithError(err).Fatal("Could not set recvbuf size")
	}

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
			veneur.Flush(metrics)
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
			for m := range parserChan {
				handlePacket(workers, m)
			}
		}()
	}

	// Read forever!
	for {
		buf := make([]byte, veneur.Config.MetricMaxLength)
		n, _, err := serverConn.ReadFromUDP(buf)
		if err != nil {
			log.WithError(err).Error("Error reading from UDP")
			continue
		}
		parserChan <- buf[:n] // TODO: termination condition for this channel?
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

	// We're ready to have a worker process this packet, so add it
	// to the work queue.
	workers[m.Digest%uint32(veneur.Config.NumWorkers)].WorkChan <- *m
}
