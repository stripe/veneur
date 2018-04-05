package proxysrv

import (
	"sync"

	"google.golang.org/grpc"
)

// clientConnMap is just a thread-safe wrapper around a map of strings to
// gRPC clients.  It also handles initializing and closing connections as
// necessary.
type clientConnMap struct {
	sync.RWMutex
	conns   map[string]*grpc.ClientConn
	options []grpc.DialOption
}

func newClientConnMap(opts ...grpc.DialOption) *clientConnMap {
	return &clientConnMap{
		conns:   make(map[string]*grpc.ClientConn),
		options: opts,
	}
}

// Return a gRPC connection for the input destination.  The ok value indicates
// if the key was found in the map.
func (m *clientConnMap) Get(dest string) (conn *grpc.ClientConn, ok bool) {
	m.RLock()
	conn, ok = m.conns[dest]
	m.RUnlock()

	return
}

// Add the destination to the map, and open a new connection to it. If the
// destination already exists, this is a no-op.
func (m *clientConnMap) Add(dest string) error {
	// If the connection already exists, just exit early
	if _, ok := m.Get(dest); ok {
		return nil
	}

	conn, err := grpc.Dial(dest, m.options...)

	m.Lock()
	_, ok := m.conns[dest]
	if !ok && err == nil {
		m.conns[dest] = conn
	}
	m.Unlock()

	if ok && err == nil {
		_ = conn.Close()
	}

	return err
}

// Delete a destination from the map and close the associated connection. This
// is a no-op if the destination doesn't exist.
func (m *clientConnMap) Delete(dest string) {
	m.Lock()

	if conn, ok := m.conns[dest]; ok {
		_ = conn.Close()
	}
	delete(m.conns, dest)

	m.Unlock()
}

// Keys returns all of the destinations in the map.
func (m *clientConnMap) Keys() []string {
	m.RLock()

	res := make([]string, 0, len(m.conns))
	for k := range m.conns {
		res = append(res, k)
	}

	m.RUnlock()
	return res
}

// Clear removes all keys from the map and closes each associated connection.
func (m *clientConnMap) Clear() {
	m.Lock()

	for _, conn := range m.conns {
		_ = conn.Close()
	}

	m.conns = make(map[string]*grpc.ClientConn)
	m.Unlock()
}
