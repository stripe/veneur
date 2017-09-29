// +build integration

package distconf

import (
	"testing"

	"time"

	"fmt"

	"sync/atomic"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/nettest"
	"github.com/signalfx/golib/zkplus"
	"github.com/stretchr/testify/assert"
)

var zkTestHost string
var zkTestPrefix string

func init() {
	C := FromLoaders([]BackingLoader{EnvLoader(), IniLoader("../.integration_test_config.ini")})
	zkTestHost = C.Str("zk.test_host", "").Get()
	zkTestPrefix = C.Str("zk.testprefix", "").Get()
	if zkTestHost == "" || zkTestPrefix == "" {
		panic("Please set zk.test_host|zk.test_host in env or .integration_test_config.ini")
	}
}

func TestZkConfIT(t *testing.T) {
	doTestWithDisconnects(t, false)
}

func TestZkConfWithKillIT(t *testing.T) {
	doTestWithDisconnects(t, true)
}

var runIndex = int32(0)

func stateMonitor(t *testing.T, zkPlusBuilder *zkplus.Builder, toToChange string, changeValues <-chan []byte) {
	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkPlusBuilder.BuildDirect()
	}))
	assert.NoError(t, err)
	defer z.Close()
	defer z.Write(toToChange, nil)
	assert.NoError(t, z.Write(toToChange, nil))
	for newValue := range changeValues {
		assert.NoError(t, z.Write(toToChange, newValue))
	}
}

func doTestWithDisconnects(t *testing.T, triggerConnKill bool) {
	const defaultTestValue = "__DEFAULT__"
	testZkConfVar := "TestZkConf" + fmt.Sprintf("%d", atomic.AddInt32(&runIndex, 1))

	var dialer nettest.TrackingDialer
	defer dialer.Close()
	zkPlusBuilder := zkplus.NewBuilder().DialZkConnector([]string{zkTestHost}, time.Second*30, dialer.DialTimeout).AppendPathPrefix(zkTestPrefix).AppendPathPrefix("config")
	changeValues := make(chan []byte)
	defer close(changeValues)
	go stateMonitor(t, zkPlusBuilder, testZkConfVar, changeValues)

	changeValues <- nil
	config := FromLoaders([]BackingLoader{BackingLoaderFunc(func() (Reader, error) {
		return Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
			return zkPlusBuilder.BuildDirect()
		}))
	})})
	s := config.Str(testZkConfVar, defaultTestValue)
	assert.Equal(t, defaultTestValue, s.Get())
	gotValue := make(chan string, 10)

	expectedOldValue := defaultTestValue
	expectedNewValue := ""
	s.Watch(func(str *Str, oldValue string) {
		assert.Equal(t, expectedOldValue, oldValue)
		assert.Equal(t, expectedNewValue, s.Get())
		gotValue <- str.Get()
	})

	updateValues := func(shouldKill bool, newValue string) {
		expectedNewValue = newValue
		if triggerConnKill && shouldKill {
			assert.NoError(t, dialer.Close())
		}

		changeValues <- []byte(newValue)
		assert.Equal(t, newValue, <-gotValue)
		assert.Equal(t, newValue, s.Get())
	}

	updateValues(true, "v1")

	expectedOldValue = "v1"
	updateValues(true, "v2")

	expectedOldValue = "v2"
	updateValues(false, "v3")
}
