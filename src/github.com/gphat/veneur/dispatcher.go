package main

import "log"

func StartDispatcher(nworkers int, workQueue chan PacketRequest) {

	// Create all of our workers.
	for i := 0; i < nworkers; i++ {
		log.Println("Starting worker", i+1)
		worker := NewWorker(i+1, workQueue)
		worker.Start()
	}
}
