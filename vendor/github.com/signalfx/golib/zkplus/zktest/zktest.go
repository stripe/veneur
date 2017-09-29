package zktest

import (
	"strings"
	"time"

	"errors"

	"sync"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/sfxtest"
	"sync/atomic"
)

// DefaultLogger is used by zktest if no logger is set
var DefaultLogger = log.Logger(log.DefaultLogger.CreateChild())

type event struct {
}

// ErrDeleteOnRoot is returned when EnsureDelete is called with a root path
var ErrDeleteOnRoot = errors.New("cannot delete on root path")

// ErrDeleteFailed is retured when EnsureDelete() is unable to ensure the delete
var ErrDeleteFailed = errors.New("delete eventually failed")

// EnsureDelete will ensure that path is deleted from zk, trying up to three times before eventually
// failing with an error
func EnsureDelete(z ZkConnSupported, path string) error {
	if path == "/" {
		return ErrDeleteOnRoot
	}
	c, _, err := z.Children(path)
	if err == nil {
		wg := sync.WaitGroup{}
		var err error
		for _, chil := range c {
			wg.Add(1)
			go func(chil string) {
				defer wg.Done()
				err = EnsureDelete(z, path+"/"+chil)
				if err != nil {
					return
				}
			}(chil)
		}
		wg.Wait()
		if err != nil {
			return err
		}
	}
	for i := 0; i < 3; i++ {
		err = z.Delete(path, -1)
		if err == nil || err == zk.ErrNoNode {
			return nil
		}
	}
	return ErrDeleteFailed
}

// MemoryZkServer can be used in the place of a zk.Conn() to unit test zk connections
type MemoryZkServer struct {
	sfxtest.ErrChecker
	root       *zkNode
	rootLock   sync.Mutex
	GlobalChan chan zk.Event
	Logger     log.Logger

	events chan event
	nextID int64

	childrenConnectionsLock sync.Mutex
	childrenConnections     map[*ZkConn]struct{}
	ChanTimeout             time.Duration
}

// ZkConn is the connection type returned from a MemoryZkConn that simulates a zk connection
type ZkConn struct {
	connectedTo   *MemoryZkServer
	events        chan zk.Event
	pathWatch     map[string]chan zk.Event
	pathWatchLock sync.Mutex
	Logger        log.Logger

	chanTimeout    time.Duration
	methodCallLock sync.Mutex

	sfxtest.ErrChecker
}

// Pretty returns a pretty print of the zk structure
func (z *MemoryZkServer) Pretty() string {
	return z.root.pretty(0)
}

func (z *zkNode) pretty(tabsize int) string {
	t := strings.Repeat("\t", tabsize)
	s := []string{(t + z.name + "->" + string(z.data))}
	for _, c := range z.children {
		s = append(s, c.pretty(tabsize+1))
	}
	return strings.Join(s, "\n")
}

type zkNode struct {
	data     []byte
	children map[string]*zkNode
	name     string
	parent   *zkNode
	stat     *zk.Stat
}

// New returns a new testing zk connection
func New() *MemoryZkServer {
	GlobalChan := make(chan zk.Event, 5)
	z := &MemoryZkServer{
		root: &zkNode{
			data:     []byte(""),
			children: make(map[string]*zkNode),
			name:     "",
			parent:   nil,
			stat:     &zk.Stat{},
		},
		GlobalChan:          GlobalChan,
		Logger:              DefaultLogger,
		events:              make(chan event),
		childrenConnections: make(map[*ZkConn]struct{}),
		ChanTimeout:         time.Second,
	}
	return z
}

// Conn satisfies the ZkConnector interface for zkplus so we can easily pass the memory zk server
// into a builder
func (z *MemoryZkServer) Conn() (ZkConnSupported, <-chan zk.Event, error) {
	return z.Connect()
}

// Connect to this server
func (z *MemoryZkServer) Connect() (*ZkConn, <-chan zk.Event, error) {
	r := &ZkConn{
		connectedTo: z,
		Logger:      log.NewContext(z.Logger).With("id", atomic.AddInt64(&z.nextID, 1)),
		events:      make(chan zk.Event, 1000),
		pathWatch:   make(map[string]chan zk.Event),
		chanTimeout: z.ChanTimeout,
	}
	z.childrenConnectionsLock.Lock()
	defer z.childrenConnectionsLock.Unlock()
	z.childrenConnections[r] = struct{}{}
	r.events <- zk.Event{
		Type:   zk.EventSession,
		State:  zk.StateConnecting,
		Server: "localhost",
	}
	r.events <- zk.Event{
		Type:   zk.EventSession,
		State:  zk.StateHasSession,
		Server: "localhost",
	}
	return r, r.events, nil
}

func (z *MemoryZkServer) addEvent(e zk.Event) {
	z.childrenConnectionsLock.Lock()
	defer z.childrenConnectionsLock.Unlock()
	for conn := range z.childrenConnections {
		conn.offerEvent(e)
	}
}

func (z *MemoryZkServer) removeConnection(c *ZkConn) {
	z.childrenConnectionsLock.Lock()
	defer z.childrenConnectionsLock.Unlock()
	delete(z.childrenConnections, c)
}

func (z *ZkConn) offerEvent(e zk.Event) {
	eventLog := log.NewContext(z.Logger).With("event", e.Path)
	z.pathWatchLock.Lock()
	defer z.pathWatchLock.Unlock()
	if z.pathWatch == nil {
		return
	}
	w, exists := z.pathWatch[e.Path]
	eventLog.Log("exists", exists, "Event on path")
	if exists {
		eventLog.Log("Firing event!")
		delete(z.pathWatch, e.Path)
		go func() {
			select {
			case w <- e:
				eventLog.Log("Event sent!")
			case <-time.After(z.chanTimeout):
			}
			close(w)
		}()

		go func() {
			select {
			case z.events <- e:
				eventLog.Log("Event sent again!")
			case <-time.After(z.chanTimeout):
			}
		}()
	}
}

func (z *zkNode) path() string {
	if z.parent == nil {
		return z.name
	}
	return z.parent.path() + "/" + z.name
}

func (z *MemoryZkServer) node(path string) (*zkNode, *zkNode) {
	parts := strings.Split(path, "/")
	parent := (*zkNode)(nil)
	at := z.root
	for _, part := range parts {
		if part == "" {
			continue
		}
		if at == nil {
			return nil, nil
		}
		nextDir := at.children[part]
		parent = at
		at = nextDir
	}
	return at, parent
}

// ZkConnSupported is the interface of zk.Conn we currently support
type ZkConnSupported interface {
	// Exists returns true if the path exists
	Exists(path string) (bool, *zk.Stat, error)
	ExistsW(path string) (bool, *zk.Stat, <-chan zk.Event, error)
	Get(path string) ([]byte, *zk.Stat, error)
	GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error)
	Children(path string) ([]string, *zk.Stat, error)
	ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error)
	Delete(path string, version int32) error
	Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error)
	Set(path string, data []byte, version int32) (*zk.Stat, error)
	Close()
}

var _ ZkConnSupported = &zk.Conn{}
var _ ZkConnSupported = &ZkConn{}

// Close sends disconnected to all waiting events and deregisteres this
// conn with the parent server
func (z *ZkConn) Close() {
	z.pathWatchLock.Lock()
	var wg sync.WaitGroup
	for _, e := range z.pathWatch {
		wg.Add(1)
		go func(e chan zk.Event) {
			defer wg.Done()
			select {
			case e <- zk.Event{
				State: zk.StateDisconnected,
			}:
			case <-time.After(z.chanTimeout):
			}
		}(e)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case z.events <- zk.Event{
			State: zk.StateDisconnected,
		}:
		case <-time.After(z.chanTimeout):
		}
	}()
	z.pathWatch = nil
	z.pathWatchLock.Unlock()
	wg.Wait()
	z.connectedTo.removeConnection(z)
}

func (z *MemoryZkServer) exists(path string) (bool, *zk.Stat, error) {
	path = fixPath(path)
	z.rootLock.Lock()
	defer z.rootLock.Unlock()
	if err := z.CheckForError("exists"); err != nil {
		return false, nil, err
	}
	at, _ := z.node(path)
	if at == nil {
		return false, nil, nil
	}
	return true, at.stat, nil
}

// Exists returns true if the path exists
func (z *ZkConn) Exists(path string) (bool, *zk.Stat, error) {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	if err := z.CheckForError("exists"); err != nil {
		return false, nil, err
	}
	return z.connectedTo.exists(path)
}

func fixPath(path string) string {
	if len(path) > 0 && path[0] != '/' {
		return "/" + path
	}
	return path
}

// ExistsW is like Exists but also sets a watch.
func (z *ZkConn) ExistsW(path string) (bool, *zk.Stat, <-chan zk.Event, error) {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	if err := z.CheckForError("exists"); err != nil {
		return false, nil, nil, err
	}
	e, s, err := z.connectedTo.exists(path)
	return e, s, z.patchWatch(path), err
}

func (z *MemoryZkServer) get(path string) ([]byte, *zk.Stat, error) {
	z.rootLock.Lock()
	defer z.rootLock.Unlock()
	if err := z.CheckForError("get"); err != nil {
		return nil, nil, err
	}
	at, _ := z.node(path)
	if at == nil {
		return nil, nil, zk.ErrNoNode
	}
	return at.data, at.stat, nil
}

// Get the bytes of a zk path
func (z *ZkConn) Get(path string) ([]byte, *zk.Stat, error) {
	if err := z.CheckForError("get"); err != nil {
		return nil, nil, err
	}
	return z.connectedTo.get(path)
}

// GetW is like Get, but also sets a watch
func (z *ZkConn) GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error) {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	if err := z.CheckForError("getw"); err != nil {
		return nil, nil, nil, err
	}
	e, s, err := z.Get(path)
	if err != nil {
		return e, s, nil, err
	}
	return e, s, z.patchWatch(path), err
}

func (z *ZkConn) patchWatch(path string) chan zk.Event {
	path = fixPath(path)
	pathWatchLogger := log.NewContext(z.Logger).With("path", path)
	pathWatchLogger.Log("Should I set a path watch?")
	z.pathWatchLock.Lock()
	defer z.pathWatchLock.Unlock()
	if z.pathWatch == nil {
		return nil
	}
	ch, exists := z.pathWatch[path]
	if !exists {
		pathWatchLogger.Log("Setting patch watch")
		ch = make(chan zk.Event)
		z.pathWatch[path] = ch
	}
	return ch
}

func (z *MemoryZkServer) children(path string) ([]string, *zk.Stat, error) {
	if err := z.CheckForError("children"); err != nil {
		return nil, nil, err
	}
	z.rootLock.Lock()
	defer z.rootLock.Unlock()
	at, _ := z.node(path)
	if at == nil {
		return nil, nil, zk.ErrNoNode
	}

	childrenNames := []string{}
	for k := range at.children {
		childrenNames = append(childrenNames, k)
	}
	return childrenNames, at.stat, nil
}

// Children gets children of a path
func (z *ZkConn) Children(path string) ([]string, *zk.Stat, error) {
	if err := z.CheckForError("children"); err != nil {
		return nil, nil, err
	}
	return z.connectedTo.children(path)
}

// ChildrenW is like children but also sets a watch
func (z *ZkConn) ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error) {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	if err := z.CheckForError("childrenw"); err != nil {
		return nil, nil, nil, err
	}
	e, s, err := z.Children(path)
	if err != nil {
		return e, s, nil, err
	}
	return e, s, z.patchWatch(path), err
}

func (z *MemoryZkServer) delete(path string, version int32) error {
	z.rootLock.Lock()
	defer z.rootLock.Unlock()
	if err := z.CheckForError("delete"); err != nil {
		return err
	}
	path = fixPath(path)
	at, parent := z.node(path)
	if at == nil {
		return zk.ErrNoNode
	}
	if version != at.stat.Version && version != -1 {
		return zk.ErrBadVersion
	}
	if len(at.children) != 0 {
		return zk.ErrNotEmpty
	}

	delete(at.parent.children, at.name)
	z.addEvent(zk.Event{
		Type:  zk.EventNodeDeleted,
		State: zk.StateConnected,
		Path:  path,
		Err:   nil,
	})

	z.addEvent(zk.Event{
		Type:  zk.EventNodeChildrenChanged,
		State: zk.StateConnected,
		Path:  parent.path(),
		Err:   nil,
	})

	return nil
}

// Delete a Zk node
func (z *ZkConn) Delete(path string, version int32) error {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	if err := z.CheckForError("delete"); err != nil {
		return err
	}
	return z.connectedTo.delete(path, version)
}

func (z *MemoryZkServer) create(path string, data []byte, flags int32, acl []zk.ACL) (string, error) {
	z.rootLock.Lock()
	defer z.rootLock.Unlock()
	path = fixPath(path)
	if err := z.CheckForError("create"); err != nil {
		return "", err
	}
	at, parent := z.node(path)
	if at != nil {
		return "", zk.ErrNodeExists
	}
	if parent == nil {
		return "", zk.ErrNoNode
	}
	name := path[len(parent.path())+1:]
	n := &zkNode{
		data:     data,
		children: make(map[string]*zkNode),
		name:     name,
		parent:   parent,
		stat:     &zk.Stat{},
	}
	parent.children[n.name] = n
	z.addEvent(zk.Event{
		Type:  zk.EventNodeCreated,
		State: zk.StateConnected,
		Path:  path,
		Err:   nil,
	})

	z.addEvent(zk.Event{
		Type:  zk.EventNodeChildrenChanged,
		State: zk.StateConnected,
		Path:  parent.path(),
		Err:   nil,
	})
	return n.path(), nil
}

// Create a Zk node
func (z *ZkConn) Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error) {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	if err := z.CheckForError("create"); err != nil {
		return "", err
	}
	return z.connectedTo.create(path, data, flags, acl)
}

func (z *MemoryZkServer) set(path string, data []byte, version int32) (*zk.Stat, error) {
	z.rootLock.Lock()
	defer z.rootLock.Unlock()
	path = fixPath(path)
	at, _ := z.node(path)
	if at == nil {
		return nil, zk.ErrNoNode
	}
	if version != at.stat.Version {
		return nil, zk.ErrBadVersion
	}
	at.data = data
	at.stat.Version++
	z.addEvent(zk.Event{
		Type:  zk.EventNodeDataChanged,
		State: zk.StateConnected,
		Path:  path,
		Err:   nil,
	})
	return at.stat, nil
}

// Set the data of a zk node
func (z *ZkConn) Set(path string, data []byte, version int32) (*zk.Stat, error) {
	z.methodCallLock.Lock()
	defer z.methodCallLock.Unlock()
	return z.connectedTo.set(path, data, version)
}
