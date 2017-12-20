// +build integration

package zktest

import (
	"testing"
	"time"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/distconf"
	"github.com/stretchr/testify/assert"
)

var zkTestHost string

func init() {
	C := distconf.FromLoaders([]distconf.BackingLoader{distconf.EnvLoader(), distconf.IniLoader("../../.integration_test_config.ini")})
	zkTestHost = C.Str("zk.test_host", "").Get()
	if zkTestHost == "" {
		panic("Please set zk.test_host in env or .integration_test_config.ini")
	}
}

func TestBasicsIT(t *testing.T) {
	t.Parallel()
	z, _, err := zk.Connect([]string{zkTestHost}, time.Second)
	defer z.Close()
	assert.NoError(t, err)
	testBasics(t, z)
}

func TestSetIT(t *testing.T) {
	t.Parallel()
	z, _, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	defer z.Close()
	testSet(t, z)
}

func TestExistsWIT(t *testing.T) {
	t.Parallel()
	z, ch, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	defer z.Close()
	testExistsW(t, z, ch)
}

func TestGetWIT(t *testing.T) {
	t.Parallel()
	z, ch, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	defer z.Close()
	testGetW(t, z, ch)
}

func TestChildrenWIT(t *testing.T) {
	t.Parallel()
	z, ch, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	defer z.Close()
	z2, _, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	testChildrenW(t, z, z2, ch)
}

func TestChildrenWNotHereIT(t *testing.T) {
	t.Parallel()
	z, ch, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	defer z.Close()
	z2, _, err := zk.Connect([]string{zkTestHost}, time.Second)
	assert.NoError(t, err)
	testChildrenWNotHere(t, z, z2, ch)
}
