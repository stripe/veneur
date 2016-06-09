package main

import (
	"flag"
	"hash/fnv"
	"net"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/stripe/veneur"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

type packet struct {
	buf []byte
	ip  string
}

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
		defer func() {
			veneur.ConsumePanic(recover())
		}()
		for t := range ticker.C {
			metrics := make([][]veneur.DDMetric, veneur.Config.NumWorkers)
			for i, w := range workers {
				log.WithFields(log.Fields{
					"worker": i,
					"tick":   t,
				}).Debug("Flushing")
				metrics = append(metrics, w.Flush(veneur.Config.Interval))
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

	packetPool := &sync.Pool{
		New: func() interface{} {
			return packet{
				buf: make([]byte, veneur.Config.MetricMaxLength),
				ip:  "",
			}
		},
	}

	// precompute digest for the "veneur.unique_sender_ips" set, since it's the
	// same every time
	veneurIPdigester := fnv.New32()
	veneurIPdigester.Write([]byte("veneur.unique_sender_ips"))
	veneurIPdigester.Write([]byte("set"))
	uniqueSenderIPDigest := veneurIPdigester.Sum32()

	// Creates N workers to handle incoming packets, parsing them,
	// hashing them and dispatching them on to workers that do the storage.
	parserChan := make(chan packet)
	for i := 0; i < veneur.Config.NumWorkers; i++ {
		go func() {
			defer func() {
				veneur.ConsumePanic(recover())
			}()
			for packetbuf := range parserChan {
				handlePacket(workers, packetbuf.buf)
				// we don't want to talk to statsd in the packet-processing code
				// path - it causes packet drops (even in the parsing loop).
				// instead, we can push the metric to a worker and let it handle
				// the IO at flush time, which has much less performance impact
				// note that all of these metrics hit a single worker, we might
				// want to have a dedicated worker for this in the future
				workers[uniqueSenderIPDigest%uint32(veneur.Config.NumWorkers)].WorkChan <- veneur.Metric{
					Name:       "veneur.unique_sender_ips",
					Digest:     uniqueSenderIPDigest,
					Value:      packetbuf.ip,
					SampleRate: 1.0,
					Type:       "set",
				}
				packetbuf.buf = packetbuf.buf[:cap(packetbuf.buf)]
				// handlePacket generates a Metric struct which contains only
				// strings, so there are no outstanding references to the byte
				// slice containing the packet
				// therefore, at this point we can return it to the pool
				packetPool.Put(packetbuf)
			}
		}()
	}

	// Read forever!
	for {
		packetbuf := packetPool.Get().(packet)
		n, addr, err := serverConn.ReadFromUDP(packetbuf.buf)
		if err != nil {
			log.WithError(err).Error("Error reading packetbuf from UDP")
			continue
		}
		packetbuf.buf = packetbuf.buf[:n]
		packetbuf.ip = addr.IP.String()
		parserChan <- packetbuf
	}
}

func handlePacket(workers []*veneur.Worker, packet []byte) {
	m, err := veneur.ParseMetric(packet)
	if err != nil {
		log.WithFields(log.Fields{
			"error":  err,
			"packet": string(packet),
		}).Error("Error parsing packet")
		veneur.Stats.Count("packet.error_total", 1, nil, 1.0)
		return
	}
	if len(veneur.Config.Tags) > 0 {
		m.Tags = append(m.Tags, veneur.Config.Tags...)
	}

	// We're ready to have a worker process this packet, so add it
	// to the work queue.
	workers[m.Digest%uint32(veneur.Config.NumWorkers)].WorkChan <- *m
}
