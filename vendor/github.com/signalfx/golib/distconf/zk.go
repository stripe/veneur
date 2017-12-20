package distconf

import (
	"fmt"

	"sync"

	"time"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/logkey"
	"github.com/signalfx/golib/pointer"
)

// ZkConn does zookeeper connections
type ZkConn interface {
	Exists(path string) (bool, *zk.Stat, error)
	Get(path string) ([]byte, *zk.Stat, error)
	GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error)
	Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error)
	Set(path string, data []byte, version int32) (*zk.Stat, error)
	ExistsW(path string) (bool, *zk.Stat, <-chan zk.Event, error)
	ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error)
	Delete(path string, version int32) error
	Close()
}

// ZkConnector creates zk connections for distconf
type ZkConnector interface {
	Connect() (ZkConn, <-chan zk.Event, error)
}

// ZkConnectorFunc wraps a dumb function to help you get a ZkConnector
type ZkConnectorFunc func() (ZkConn, <-chan zk.Event, error)

// Connect to Zk by calling itself()
func (z ZkConnectorFunc) Connect() (ZkConn, <-chan zk.Event, error) {
	return z()
}

type callbackMap struct {
	mu        sync.Mutex
	callbacks map[string][]backingCallbackFunction
}

func (c *callbackMap) add(key string, val backingCallbackFunction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks[key] = append(c.callbacks[key], val)
}

func (c *callbackMap) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.callbacks)
}

func (c *callbackMap) get(key string) []backingCallbackFunction {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, exists := c.callbacks[key]
	if exists {
		return r
	}
	return []backingCallbackFunction{}
}

func (c *callbackMap) copy() map[string][]backingCallbackFunction {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make(map[string][]backingCallbackFunction, len(c.callbacks))
	for k, v := range c.callbacks {
		ret[k] = []backingCallbackFunction{}
		for _, c := range v {
			ret[k] = append(ret[k], c)
		}
	}
	return ret
}

type atomicDuration struct {
	mu                sync.Mutex
	refreshRetryDelay time.Duration
}

func (a *atomicDuration) set(t time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refreshRetryDelay = t
}

func (a *atomicDuration) get() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshRetryDelay
}

type zkConfig struct {
	conn       ZkConn
	eventChan  <-chan zk.Event
	shouldQuit chan struct{}

	rootLogger   log.Logger
	callbacks    callbackMap
	refreshDelay atomicDuration
}

func (back *zkConfig) configPath(key string) string {
	return fmt.Sprintf("%s", key)
}

// Get returns the config value from zookeeper
func (back *zkConfig) Get(key string) ([]byte, error) {
	back.rootLogger.Log(logkey.ZkMethod, "get", logkey.ZkPath, key)
	pathToFetch := back.configPath(key)
	bytes, _, _, err := back.conn.GetW(pathToFetch)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, nil
		}
		return nil, errors.Annotatef(err, "cannot load zk node %s", pathToFetch)
	}
	return bytes, nil
}

func (back *zkConfig) Write(key string, value []byte) error {
	back.rootLogger.Log(logkey.ZkMethod, "write", logkey.ZkPath, key)
	path := back.configPath(key)
	exists, stat, err := back.conn.Exists(path)
	if err != nil {
		return err
	}
	if !exists {
		if value == nil {
			return nil
		}
		_, err := back.conn.Create(path, value, 0, zk.WorldACL(zk.PermAll))
		return errors.Annotatef(err, "cannot create path %s", path)
	}
	if value == nil {
		err = back.conn.Delete(path, stat.Version)
	} else {
		stat, err = back.conn.Set(path, value, stat.Version)
	}

	return errors.Annotatef(err, "cannot change path %s", path)
}

func (back *zkConfig) Watch(key string, callback backingCallbackFunction) error {
	back.rootLogger.Log(logkey.ZkMethod, "watch", logkey.ZkPath, key)
	path := back.configPath(key)
	_, _, _, err := back.conn.ExistsW(path)
	if err != nil {
		return errors.Annotatef(err, "cannot watch path %s", path)
	}
	back.callbacks.add(path, callback)

	return nil
}

func (back *zkConfig) Close() {
	close(back.shouldQuit)
	back.conn.Close()
}

func (back *zkConfig) logInfoState(logger log.Logger, e zk.Event) bool {
	if e.State == zk.StateDisconnected {
		logger.Log("Disconnected from zookeeper.  Will attempt to remake connection.")
		return true
	}
	if e.State == zk.StateConnecting {
		logger.Log("Server is now attempting to reconnect.")
		return true
	}
	return false
}

func eventToString(ev zk.Event) string {
	return fmt.Sprintf("%s %s %s", ev.Path, ev.State.String(), ev.Type.String())
}

func (back *zkConfig) drainEventChan(functionLogger log.Logger) {
	drainContext := log.NewContext(functionLogger).With(logkey.Func, "drainEventChan")
	defer drainContext.Log("Draining done")
	for {
		drainContext.Log("Blocking with event")
		select {
		case e := <-back.eventChan:
			eventContext := drainContext.With(logkey.ZkEvent, eventToString(e))
			eventContext.Log("event seen")
			back.logInfoState(eventContext, e)
			if e.State == zk.StateHasSession {
				eventContext.Log("Server now has a session.")
				back.refreshWatches(eventContext)
				continue
			}
			if e.Path == "" {
				continue
			}
			if len(e.Path) > 0 && e.Path[0] == '/' {
				e.Path = e.Path[1:]
			}
			{
				eventContext.Log(logkey.ArrLen, back.callbacks.len(), "change state")
				for _, c := range back.callbacks.get(e.Path) {
					c(e.Path)
				}
			}
			eventContext.Log("reregistering watch")

			// Note: return value currently ignored.  Not sure what to do about it
			log.IfErr(eventContext, back.reregisterWatch(e.Path, eventContext))
			eventContext.Log("reregistering watch finished")
		case <-back.shouldQuit:
			drainContext.Log("quit message seen")
			return
		}
	}
}

func (back *zkConfig) refreshWatches(functionLogger log.Logger) {
	for path, callbacks := range back.callbacks.copy() {
		pathLogger := log.NewContext(functionLogger).With(logkey.ZkPath, path)
		for _, c := range callbacks {
			c(path)
		}
		for {
			err := back.reregisterWatch(path, pathLogger)
			if err == nil {
				break
			}
			pathLogger.Log(log.Err, err, "Error reregistering watch")
			time.Sleep(back.refreshDelay.get())
		}
	}
}

func (back *zkConfig) setRefreshDelay(refreshDelay time.Duration) {
	back.refreshDelay.set(refreshDelay)
}

func (back *zkConfig) reregisterWatch(path string, logger log.Logger) error {
	logger = log.NewContext(logger).With(logkey.ZkPath, path)
	logger.Log("Reregistering watch")
	_, _, _, err := back.conn.ExistsW(path)
	if err != nil {
		logger.Log(log.Err, err, "Unable to reregistering watch")
		return errors.Annotatef(err, "unable to reregister watch for node %s", path)
	}
	return nil
}

// ZkConfig configures optional settings about a zk distconf
type ZkConfig struct {
	Logger log.Logger
}

// DefaultZkConfig is the configuration used by new Zk readers if any config parameter is unset
var DefaultZkConfig = &ZkConfig{
	Logger: DefaultLogger,
}

// Zk creates a zookeeper readable backing
func Zk(zkConnector ZkConnector, conf *ZkConfig) (ReaderWriter, error) {
	conf = pointer.FillDefaultFrom(conf, DefaultZkConfig).(*ZkConfig)
	ret := &zkConfig{
		rootLogger: log.NewContext(conf.Logger).With(logkey.DistconfBacking, "zk"),
		shouldQuit: make(chan struct{}),
		callbacks: callbackMap{
			callbacks: make(map[string][]backingCallbackFunction),
		},
		refreshDelay: atomicDuration{
			refreshRetryDelay: time.Millisecond * 500,
		},
	}
	var err error
	ret.conn, ret.eventChan, err = zkConnector.Connect()
	if err != nil {
		return nil, errors.Annotate(err, "cannot create zk connection")
	}
	go ret.drainEventChan(conf.Logger)
	return ret, nil
}
