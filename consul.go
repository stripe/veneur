package veneur

import (
	"errors"
	"fmt"

	"github.com/hashicorp/consul/api"
)

// Consul is a Discoverer that uses Consul to find
// healthy instances of a given name.
type Consul struct {
	ConsulHealth *api.Health
}

// NewConsul creates a new instance of a Consul Discoverer
func NewConsul(config *api.Config) (*Consul, error) {
	consulClient, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &Consul{
		ConsulHealth: consulClient.Health(),
	}, nil
}

// GetDestinationsForService updates the list of destinations based on healthy nodes
// found via Consul.  It returns destinations in the form "<host>:<port>".
func (c *Consul) GetDestinationsForService(serviceName string) ([]string, error) {
	serviceEntries, _, err := c.ConsulHealth.Service(serviceName, "", true, &api.QueryOptions{})
	if err != nil {
		return nil, err
	}

	numHosts := len(serviceEntries)
	if numHosts < 1 {
		return nil, errors.New("Received no hosts from Consul")
	}
	// Make a slice to hold our returned hosts
	hosts := make([]string, numHosts)
	for index, se := range serviceEntries {
		hosts[index] = fmt.Sprintf("%s:%d", se.Node.Address, se.Service.Port)
	}

	return hosts, nil
}
