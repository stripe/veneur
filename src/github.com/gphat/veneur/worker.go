package main

import (
	"log"
	"time"
)

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker will receive work from.
func NewWorker(id int, workQueue chan PacketRequest) Worker {
	// Create, and return the worker.
	worker := Worker{
		ID:       id,
		Work:     workQueue,
		Counters: make(map[string]int),
		QuitChan: make(chan bool)}

	return worker
}

type Worker struct {
	ID       int
	Work     chan PacketRequest
	Counters map[string]int
	QuitChan chan bool
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w Worker) Start() {
	ticker := time.NewTicker(time.Duration(10) * time.Second)
	go func() {
		for t := range ticker.C {
			for k, v := range w.Counters {
				log.Printf("Ticker %d Counter %q at %d\n", t, k, v)
			}
		}
	}()

	go func() {
		for {
			select {
			case work := <-w.Work:
				// Receive a work request.
				log.Printf("worker%d: Received work request: '%s'\n", w.ID, work.Packet)
				m, err := ParseMetric(work.Packet)
				if err != nil {
					log.Printf("Invalid packet, %q", err)
					return
				}

				switch m.Type {
				case "c":
					log.Printf("Got counter %q", m.Name)
					w.Counters[m.Name]++
				}

			case <-w.QuitChan:
				// We have been asked to stop.
				log.Printf("worker%d stopping\n", w.ID)
				return
			}
		}
	}()
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w Worker) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}
