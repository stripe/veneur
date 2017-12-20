package maestro

import (
	"encoding/json"
	"fmt"

	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/safeexec"
)

// Ships are the location of an instance
type Ships struct {
	IP string
}

// Port is the port a service is running in.  Can sometimes be port ranges...
type Port interface{}

// EnvVal is the value in the instance's shell env
type EnvVal interface{}

// Instance running code
type Instance struct {
	Ports map[string]Port
	Ship  string
}

// Service running in maestro
type Service struct {
	Instances map[string]Instance
	Env       map[string]EnvVal
}

// A Config is our maestro config
type Config struct {
	Name     string
	Ships    map[string]Ships
	Services map[string]Service
}

// ShipsForService is a helper that gets the connections for a service inside a config
func (c *Config) ShipsForService(name string) []string {
	zkInstances := c.Services[name].Instances
	inst := make([]string, 0, len(zkInstances))
	for _, i := range zkInstances {
		zkShip := c.Ships[i.Ship]
		c := fmt.Sprintf("%s:%d", zkShip.IP, int64(i.Ports["client"].(float64)))
		inst = append(inst, c)
	}
	return inst
}

// ArbitraryInstance returns a (possibly random) instance in a map, or panics if the map is empty
func ArbitraryInstance(m map[string]Instance) Instance {
	for _, v := range m {
		return v
	}
	panic("I panic if the map is empty")
}

// Loader can load a maestro config execing out to the python module and returning its json
// object.
type Loader struct {
	ExecFunc       func(name string, stdin string, args ...string) (string, string, error)
	PythonLocation string
}

// LoadError is returned when Load() fails, including extra information about the python error.
type LoadError struct {
	error
	stdout string
	stderr string
}

func (l *LoadError) Error() string {
	return fmt.Sprintf("%s %s %s", l.error, l.stdout, l.stderr)
}

// Load a maestro config, shelling out to python
func (l *Loader) Load(filename string) (*Config, error) {
	f := l.ExecFunc
	if f == nil {
		f = safeexec.Execute
	}
	pythonLocation := l.PythonLocation
	if pythonLocation == "" {
		pythonLocation = "python"
	}
	stdout, stderr, err := f(pythonLocation, filename, "-c", "import maestro.__main__ as maestro, sys, json;print json.dumps(maestro.load_config_from_file(sys.stdin.readline().strip()))")
	if err != nil {
		return nil, &LoadError{error: err, stdout: stdout, stderr: stderr}
	}
	var conf Config

	if err = json.Unmarshal([]byte(stdout), &conf); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal to stdout")
	}
	return &conf, nil
}
