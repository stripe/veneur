// Package gojavahash implements a ServerSelector for gomemcache that provides
// hashing that's compatible with SpyMemcached's native (e.g. java) hashing.
// This is based very closely on the excellent https://github.com/thatguystone/gomcketama
package gojavahash

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	mc "github.com/signalfx/gomemcache/memcache"
)

const (
	numReps = 160
)

type info struct {
	hash int
	addr net.Addr
}

type infoSlice []info

// JavaServerSelector implements gomemcache's ServerSelector using java hash equiv
type JavaServerSelector struct {
	lock     sync.RWMutex
	addrs    []net.Addr
	ring     infoSlice
	numReps  int
	lookuper func(string) (net.Addr, error)
}

func (s infoSlice) Len() int {
	return len(s)
}

func (s infoSlice) Less(i, j int) bool {
	return s[i].hash < s[j].hash
}

func (s infoSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// AddServer adds a server to the ketama continuum.
func (ks *JavaServerSelector) AddServer(server string) error {
	addr, keys, err := ks.prepServer(server)
	if err != nil {
		return err
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()
	for _, key := range keys {
		k := hash(key)
		ks.ring = append(ks.ring, info{
			hash: k,
			addr: addr,
		})
	}

	sort.Sort(ks.ring)
	ks.addrs = append(ks.addrs, addr)

	return nil
}

func hash(key string) int {
	h := int32(0)
	for i := 0; i < len(key); i++ {
		h = 31*h + int32(key[i])
	}
	return int(h) & 0xffffffff
}

// PickServer returns the server address that a given item should be written
// to.
func (ks *JavaServerSelector) PickServer(key string) (net.Addr, error) {
	if len(ks.addrs) == 0 {
		return nil, mc.ErrNoServers
	}

	if len(ks.addrs) == 1 {
		return ks.addrs[0], nil
	}

	h := hash(key)

	i := sort.Search(len(ks.ring), func(i int) bool {
		return ks.ring[i].hash >= h
	})

	if i == len(ks.ring) {
		i = 0
	}

	return ks.ring[i].addr, nil
}

// Each loops through all registered servers, calling the given function.
func (ks *JavaServerSelector) Each(f func(net.Addr) error) error {
	ks.lock.RLock()
	defer ks.lock.RUnlock()
	for _, a := range ks.addrs {
		if err := f(a); nil != err {
			return err
		}
	}
	return nil
}

// New creates a new memcache client with the given servers, using ketama as
// the ServerSelector. This functions exactly like gomemcache's New().
func New(server ...string) (*mc.Client, error) {
	ks := &JavaServerSelector{}

	for _, s := range server {
		err := ks.AddServer(s)
		if err != nil {
			return nil, err
		}
	}

	return mc.NewFromSelector(ks), nil
}

func lookup(server string) (a net.Addr, err error) {
	if strings.Contains(server, "/") {
		a, err = net.ResolveUnixAddr("unix", server)
	} else {
		a, err = net.ResolveTCPAddr("tcp", server)
	}

	return
}

func (ks *JavaServerSelector) prepServer(server string) (a net.Addr, keys []string, err error) {
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		return
	}

	if ks.lookuper == nil {
		ks.lookuper = lookup
	}

	a, err = ks.lookuper(server)
	if err != nil {
		return
	}

	key := fmt.Sprintf("%s:%s", host, port)
	if !strings.HasPrefix(a.String(), host) {
		key = fmt.Sprintf("%s/%s", host, a.String())
	}
	if ks.numReps == 0 {
		ks.numReps = numReps
	}

	for i := 0; i < ks.numReps; i++ {
		keys = append(keys, fmt.Sprintf("%s-%d", key, i))
	}

	return
}
