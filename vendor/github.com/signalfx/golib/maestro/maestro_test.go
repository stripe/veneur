package maestro

import (
	"testing"

	"fmt"
	"github.com/stretchr/testify/assert"
)

type envMap map[string]string

func (e envMap) get(key string) string {
	r, exist := e[key]
	if !exist {
		return ""
	}
	return r
}

func asserErrorEq(t *testing.T, e error, f func() (string, error)) {
	_, err := f()
	assert.Equal(t, e, err)
}

func assertStrEq(t *testing.T, s string, f func() (string, error)) {
	r, err := f()
	assert.NoError(t, err)
	assert.Equal(t, s, r)
}

var i = 0

func testPath(t *testing.T, e envMap, err error, key string, f func() (string, error)) {
	asserErrorEq(t, err, f)
	v := fmt.Sprintf("%dhello", i)
	e[key] = v
	i++
	assertStrEq(t, v, f)
}

func TestMaestro(t *testing.T) {
	i = 0
	e := make(envMap)
	m := New(e.get)

	assert.Equal(t, "local", m.GetEnvironmentName())
	e["MAESTRO_ENVIRONMENT_NAME"] = "test"
	assert.Equal(t, "test", m.GetEnvironmentName())

	_, err := m.GetPort("client")
	assert.Error(t, err)
	testPath(t, e, ErrNoServiceName, "SERVICE_NAME", m.GetServiceName)

	_, err = m.GetPort("client")
	assert.Error(t, err)

	_, err = m.GetSpecificPort("service", "container", "port")
	assert.Error(t, err)

	testPath(t, e, ErrContainerHostAddrNotDefined, "CONTAINER_HOST_ADDRESS", m.GetContainerHostAddress)
	testPath(t, e, ErrNoContainerName, "CONTAINER_NAME", m.GetContainerName)

	_, err = m.GetSpecificHost("service", "container")
	assert.Error(t, err)

	e["ZOOKEEPER_INSTANCES"] = "INST1,inst2"
	e["ZOOKEEPER_INST1_HOST"] = "host1"
	e["ZOOKEEPER_INST1_CLIENT_PORT"] = "123"
	e["ZOOKEEPER_INST2_HOST"] = "host2"
	e["ZOOKEEPER_INST2_CLIENT_PORT"] = "124"

	e["0HELLO_2HELLO_CLIENT_INTERNAL_PORT"] = "125"

	assert.Equal(t, []string{}, m.getServiceInstanceNames("abc"))
	assert.Equal(t, []string{"INST1", "inst2"}, m.getServiceInstanceNames("ZOOKEEPER"))

	assert.Equal(t, []string{"host1:123", "host2:124"}, m.GetNodeList("ZOOKEEPER", []string{"client"}))
	p, err := m.GetPort("client")
	assert.NoError(t, err)
	assert.Equal(t, uint16(125), p)

	e["BADSERVICE_INSTANCES"] = "INST1,inst3"
	e["BADSERVICE_INST3_HOST"] = "host3"

	assert.Equal(t, []string{"host3"}, m.GetNodeList("BADSERVICE", []string{"badclient"}))

}
