// +build integration

package zkplus

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/distconf"
	"github.com/stretchr/testify/assert"
)

var zkTestHost string

func init() {
	C := distconf.FromLoaders([]distconf.BackingLoader{distconf.EnvLoader(), distconf.IniLoader("../.integration_test_config.ini")})
	zkTestHost = C.Str("zk.test_host", "").Get()
	if zkTestHost == "" {
		panic("Please set zk.test_host|zk.test_host in env or .integration_test_config.ini")
	}
}

var conns = []net.Conn{}

func onDemandCloseDialer(network, address string, timeout time.Duration) (net.Conn, error) {
	n, err := net.DialTimeout(network, address, timeout)
	if err != nil {
		return n, err
	}

	conns = append(conns, n)
	return n, nil
}

var _ zk.Dialer = onDemandCloseDialer

func randomString(n int) string {
	rand.Seed(time.Now().UnixNano())
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func TestZkPlusIT(t *testing.T) {
	zkPlusParent, err := NewBuilder().DialZkConnector([]string{zkTestHost}, time.Second*3, onDemandCloseDialer).Build()
	assert.NoError(t, err)
	zkPlusParent.Create("/testing", []byte{}, 0, zk.WorldACL(zk.PermAll))
	defer zkPlusParent.Close()
	defer zkPlusParent.Delete("/testing", 0)

	zkPlusChild, err := NewBuilder().ZkPlus(zkPlusParent).AppendPathPrefix("testing").Build()
	assert.NoError(t, err)
	defer zkPlusChild.Close()

	randomString := randomString(5)

	path, err := zkPlusParent.Create("/testing/"+randomString, []byte("hello"), zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)
	assert.Equal(t, "/testing/"+randomString, path)
	b, _, err := zkPlusChild.Get("/" + randomString)
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(b))
	assert.Equal(t, 2, len(conns))
	assert.NoError(t, conns[0].Close())
	assert.NoError(t, conns[1].Close())
	fmt.Printf("Closed\n")
	b, _, err = zkPlusChild.Get("/" + randomString)
	fmt.Printf("Closing size=%d\n", len(conns))
	assert.Error(t, err)
	assert.Equal(t, "", string(b))
}
