package main

import (
	"flag"
	"net"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/stripe/veneur"
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

	if veneur.Config.Debug {
		log.SetLevel(log.DebugLevel)
		log.WithField("config", veneur.Config).Debug("Starting with config")
	}

	veneur.InitSentry()
	defer func() {
		veneur.ConsumePanic(recover())
	}()
	veneur.InitStats()

	// Start the dispatcher.
	log.WithField("number", veneur.Config.NumWorkers).Info("Starting workers")
	workers := make([]*veneur.Worker, veneur.Config.NumWorkers)
	for i := 0; i < veneur.Config.NumWorkers; i++ {
		worker := veneur.NewWorker(i+1, veneur.Config.Percentiles, veneur.Config.HistCounters, veneur.Config.SetSize, veneur.Config.SetAccuracy)
		worker.Start()
		workers[i] = worker
	}

	serverAddr, err := net.ResolveUDPAddr("udp", veneur.Config.UDPAddr)
	if err != nil {
		log.WithError(err).Fatal("Error resolving address")
	}

	server := veneur.Server{
		Workers:    workers,
		Stats:      veneur.Stats,
		Hostname:   veneur.Config.Hostname,
		Tags:       veneur.Config.Tags,
		DDHostname: veneur.Config.APIHostname,
		DDAPIKey:   veneur.Config.Key,
	}

	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, veneur.Config.MetricMaxLength)
		},
	}

	// Creates N workers to handle incoming packets, parsing them,
	// hashing them and dispatching them on to workers that do the storage.
	parserChan := make(chan []byte)
	for i := 0; i < veneur.Config.NumWorkers; i++ {
		go func() {
			defer func() {
				veneur.ConsumePanic(recover())
			}()
			for packet := range parserChan {
				server.HandlePacket(packet, packetPool)
			}
		}()
	}

	// Read forever!
	for i := 0; i < veneur.Config.NumReaders; i++ {
		go func() {
			defer func() {
				veneur.ConsumePanic(recover())
			}()

			// each goroutine gets its own socket
			// if the sockets support SO_REUSEPORT, then this will cause the
			// kernel to distribute datagrams across them, for better read
			// performance
			log.WithField("address", veneur.Config.UDPAddr).Info("UDP server listening")
			serverConn, err := veneur.NewSocket(serverAddr, veneur.Config.ReadBufferSizeBytes)
			if err != nil {
				// if any goroutine fails to create the socket, we can't really
				// recover, so we just blow up
				// this probably indicates a systemic issue, eg lack of
				// SO_REUSEPORT support
				log.WithError(err).Fatal("Error listening for UDP")
			}

			for {
				buf := packetPool.Get().([]byte)
				n, _, err := serverConn.ReadFrom(buf)
				if err != nil {
					log.WithError(err).Error("Error reading from UDP")
					continue
				}
				parserChan <- buf[:n] // TODO: termination condition for this channel?
			}
		}()
	}

	ticker := time.NewTicker(veneur.Config.Interval)
	log.WithField("interval", veneur.Config.Interval).Info("Starting flush loop")
	for range ticker.C {
		server.Flush(veneur.Config.Interval, veneur.Config.FlushLimit)
	}
}
