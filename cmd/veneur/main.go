package main

import (
	"flag"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func main() {
	flag.Parse()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	conf, err := veneur.ReadConfig(*configFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error reading config file")
	}
	server, err := veneur.NewFromConfig(conf)
	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize server")
	}
	defer func() {
		server.ConsumePanic(recover())
	}()

	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, conf.MetricMaxLength)
		},
	}

	// Creates N workers to handle incoming packets, parsing them,
	// hashing them and dispatching them on to workers that do the storage.
	parserChan := make(chan []byte)
	for i := 0; i < conf.NumWorkers; i++ {
		go func() {
			defer func() {
				server.ConsumePanic(recover())
			}()
			for packet := range parserChan {
				server.HandlePacket(packet, packetPool)
			}
		}()
	}

	// Read forever!
	for i := 0; i < conf.NumReaders; i++ {
		go func() {
			defer func() {
				server.ConsumePanic(recover())
			}()
			server.ReadSocket(packetPool, parserChan)
		}()
	}

	ticker := time.NewTicker(conf.Interval)
	for range ticker.C {
		server.Flush(conf.Interval, conf.FlushLimit)
	}
}
