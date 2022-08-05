package destinations

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/proxy/connect"
	"stathat.com/c/consistent"
)

type Destinations interface {
	Add(ctx context.Context, destinations []string)
	Clear()
	Get(key string) (connect.Destination, error)
	Size() int
	Wait()
}

var _ Destinations = &destinations{}
var _ connect.DestinationHash = &destinations{}

type destinations struct {
	connecter           connect.Connect
	connectionWaitGroup sync.WaitGroup
	destinations        map[string]connect.Destination
	destinationsHash    *consistent.Consistent
	logger              *logrus.Entry
	mutex               sync.RWMutex
}

// Create a new set of destinations to forward metrics to.
func Create(
	connecter connect.Connect, logger *logrus.Entry,
) Destinations {
	return &destinations{
		connecter:        connecter,
		destinations:     map[string]connect.Destination{},
		destinationsHash: consistent.New(),
		logger:           logger,
	}
}

// Adds and connects to a destination. Destinations will be automatically
// removed if the connection to them closes.
func (d *destinations) Add(
	ctx context.Context, destinations []string,
) {
	// Filter new destinations.
	addedDestinations := []string{}
	func() {
		d.mutex.RLock()
		defer d.mutex.RUnlock()

		for _, destination := range destinations {
			_, ok := d.destinations[destination]
			if !ok {
				addedDestinations = append(addedDestinations, destination)
			}
		}
	}()

	// Connect to each new destination in parallel.
	waitGroup := sync.WaitGroup{}
	for _, addedDestination := range addedDestinations {
		waitGroup.Add(1)
		go func(address string) {
			defer waitGroup.Done()

			d.connectionWaitGroup.Add(1)
			destination, err := d.connecter.Connect(ctx, address, d)
			if err != nil {
				return
			}
			d.addDestination(address, destination)
		}(addedDestination)
	}
	waitGroup.Wait()
}

// Adds a destination to the consistent hash.
func (d *destinations) addDestination(
	address string, destination connect.Destination,
) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	oldDestination, ok := d.destinations[address]
	if ok {
		d.logger.WithField("destination", address).Error("duplicate destination")
		oldDestination.Close()
	}
	d.destinations[address] = destination
	d.destinationsHash.Add(address)
}

// Removes a destination from the consistent hash.
func (d *destinations) RemoveDestination(address string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	_, ok := d.destinations[address]
	if !ok {
		return
	}
	d.destinationsHash.Remove(address)
	delete(d.destinations, address)
}

func (d *destinations) ConnectionClosed() {
	d.connectionWaitGroup.Done()
}

// Removes all destinations, and closes connections to them.
func (d *destinations) Clear() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.destinationsHash.Set([]string{})
	for address, destination := range d.destinations {
		delete(d.destinations, address)
		destination.Close()
	}
}

// Gets a destination for a given key.
func (d *destinations) Get(key string) (connect.Destination, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	destinationAddress, err := d.destinationsHash.Get(key)
	if err != nil {
		return nil, err
	}
	destination, ok := d.destinations[destinationAddress]
	if !ok {
		return nil, fmt.Errorf("unknown destination: %s", destinationAddress)
	}
	return destination, nil
}

// Returns the current number of destinations.
func (d *destinations) Size() int {
	return len(d.destinations)
}

// Waits for all current connections to be closed and removed.
func (d *destinations) Wait() {
	d.connectionWaitGroup.Wait()
}
