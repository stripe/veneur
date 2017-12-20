package maestro

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/signalfx/golib/errors"
)

// Maestro is the golang client for https://github.com/signalfuse/maestro-ng
type Maestro struct {
	// Stub for os.Getenv
	osGetenv func(string) string
}

// New creates a new maestro client, using the given function to get env variables.  You usually
// want to call New(os.Getenv)
func New(osGetenv func(string) string) *Maestro {
	return &Maestro{
		osGetenv: osGetenv,
	}
}

// GetEnvironmentName returns the name of the environment the container calling this in a part of
func (m *Maestro) GetEnvironmentName() string {
	val := m.osGetenv("MAESTRO_ENVIRONMENT_NAME")
	if val == "" {
		return "local"
	}
	return val
}

// ErrContainerHostAddrNotDefined is returned when GetContainerHostAddress is not defined
var ErrContainerHostAddrNotDefined = errors.New("container host address was not defined")

// GetContainerHostAddress returns the publicly-addressable IP address of the host of the container
func (m *Maestro) GetContainerHostAddress() (string, error) {
	address := m.osGetenv("CONTAINER_HOST_ADDRESS")
	if address == "" {
		return "", ErrContainerHostAddrNotDefined
	}
	return address, nil
}

// ErrNoServiceName is returned when GetServiceName is not defined
var ErrNoServiceName = errors.New("service name was not defined")

// GetServiceName returns the service name of the container calling it
func (m *Maestro) GetServiceName() (string, error) {
	address := m.osGetenv("SERVICE_NAME")
	if address == "" {
		return "", ErrNoServiceName
	}
	return address, nil
}

// ErrNoContainerName is returned when GetContainerName is not defined
var ErrNoContainerName = errors.New("container name was not defined")

// GetContainerName returns the name of the container calling it
func (m *Maestro) GetContainerName() (string, error) {
	address := m.osGetenv("CONTAINER_NAME")
	if address == "" {
		return "", ErrNoContainerName
	}
	return address, nil
}

// ErrPortNotDefined is returned when the port cannot be found
var ErrPortNotDefined = errors.New("port env not defined")

// GetSpecificExposedPort returns the exposed (internal) port number of a specific port of a
// specific container from a given service.
func (m *Maestro) GetSpecificExposedPort(service string, container string, port string) (uint16, error) {
	return m.getPortHelper(service, container, port, "INTERNAL_PORT")
}

// ErrNoSpecificHostDefined is returned when GetSpecificHost cannot find the host in the container
var ErrNoSpecificHostDefined = errors.New("no host defined for container of service")

// GetSpecificHost returns the hostname/address of a specific container/instance of the given service
func (m *Maestro) GetSpecificHost(service string, container string) (string, error) {
	envKey := fmt.Sprintf("%s_%s_HOST", m.toEnvVarName(service), m.toEnvVarName(container))
	envVal := m.osGetenv(envKey)
	if envVal == "" {
		return "", ErrNoSpecificHostDefined
	}
	return envVal, nil
}

// GetPort returns the exposed (internal) port number for the given port
func (m *Maestro) GetPort(name string) (uint16, error) {
	sname, err := m.GetServiceName()
	if err != nil {
		return 0, errors.Annotate(err, "cannot load service for port")
	}
	cname, err := m.GetContainerName()
	if err != nil {
		return 0, errors.Annotate(err, "cannot load container name for port")
	}
	return m.GetSpecificExposedPort(sname, cname, name)
}

// GetSpecificPort returns the external port number of a specific port of a specific
// container from a given service
func (m *Maestro) GetSpecificPort(service string, container string, port string) (uint16, error) {
	return m.getPortHelper(service, container, port, "PORT")
}

func (m *Maestro) getPortHelper(service string, container string, port string, key string) (uint16, error) {
	envKey := fmt.Sprintf("%s_%s_%s_%s", m.toEnvVarName(service), m.toEnvVarName(container), m.toEnvVarName(port), key)
	envVal := m.osGetenv(strings.ToUpper(envKey))
	if envVal == "" {
		return 0, ErrPortNotDefined
	}
	portInt, err := strconv.ParseUint(envVal, 10, 16)
	return uint16(portInt), err
}

// GetNodeList builds a list of nodes for the given service from the environment,
// eventually adding the ports from the list of port names. The resulting
// entries will be of the form 'host[:port1[:port2]]' and sorted by container name.
func (m *Maestro) GetNodeList(service string, ports []string) []string {
	nodes := []string{}
	for _, container := range m.getServiceInstanceNames(service) {
		node, err := m.GetSpecificHost(service, container)
		if err != nil {
			continue
		}
		for _, port := range ports {
			p, err := m.GetSpecificPort(service, container, port)
			if err != nil {
				continue
			}
			node = fmt.Sprintf("%s:%d", node, p)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func (m *Maestro) toEnvVarName(name string) string {
	return strings.ToUpper(string(regexp.MustCompile(`[^\w]`).ReplaceAll([]byte(name), []byte("_"))))
}

func (m *Maestro) getServiceInstanceNames(service string) []string {
	key := fmt.Sprintf("%s_INSTANCES", m.toEnvVarName(service))
	val := m.osGetenv(key)
	if val == "" {
		return []string{}
	}
	return strings.Split(val, ",")
}
