package main

import (
	"flag"
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

	server, err := veneur.NewFromConfig(veneur.Config)
	if err != nil {
		log.WithError(err).Fatal("Could not initialize server")
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
			server.ReadSocket(packetPool, parserChan)
		}()
	}

	ticker := time.NewTicker(veneur.Config.Interval)
	log.WithField("interval", veneur.Config.Interval).Info("Starting flush loop")
	for range ticker.C {
		server.Flush(veneur.Config.Interval, veneur.Config.FlushLimit)
	}
}
