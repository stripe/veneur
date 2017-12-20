package disco

import (
	"crypto/rand"
	"io"

	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"expvar"

	"strings"
	"sync"

	"bytes"
	"encoding/binary"
	"sort"

	"github.com/signalfx/golib/errors"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/logkey"
	"github.com/signalfx/golib/pointer"
	"github.com/signalfx/golib/zkplus"
)

// ServiceInstance defines a remote service and is similar to
// https://curator.apache.org/apidocs/org/apache/curator/x/discovery/ServiceInstanceBuilder.html
type ServiceInstance struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Payload             interface{} `json:"payload,omitempty"`
	Address             string      `json:"address"`
	Port                uint16      `json:"port"`
	RegistrationTimeUTC int64       `json:"registrationTimeUTC"`
	SslPort             *uint16     `json:"sslPort"`
	ServiceType         string      `json:"serviceType"`
	URISpec             *string     `json:"uriSpec"`
}

func (s *ServiceInstance) uniqueHash() []byte {
	buf := new(bytes.Buffer)
	errors.PanicIfErrWrite(buf.Write([]byte(s.ID)))
	log.IfErr(log.Panic, binary.Write(buf, binary.BigEndian, s.RegistrationTimeUTC))
	return buf.Bytes()
}

// DialString is a string that net.Dial() can accept that will connect to this service's Port
func (s *ServiceInstance) DialString() string {
	return fmt.Sprintf("%s:%d", s.Address, s.Port)
}

// ChangeWatch is a callback you can register on a service that is executed whenever the service's
// instances change
type ChangeWatch func()

// Service is a set of ServiceInstance that describe a discovered service
type Service struct {
	services      atomic.Value // []ServiceInstance
	name          string
	ignoreUpdates int64

	stateLog  log.Logger
	watchLock sync.Mutex
	watches   []ChangeWatch
}

// ZkConn does zookeeper connections
type ZkConn interface {
	GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error)
	Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error)
	ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error)
	ExistsW(path string) (bool, *zk.Stat, <-chan zk.Event, error)
	Delete(path string, version int32) error
	Close()
}

// ZkConnCreator creates Zk connections for disco to use.
type ZkConnCreator interface {
	Connect() (ZkConn, <-chan zk.Event, error)
}

// ZkConnCreatorFunc gives you a ZkConnCreator out of a function
type ZkConnCreatorFunc func() (ZkConn, <-chan zk.Event, error)

// Connect to a zookeeper endpoint
func (z ZkConnCreatorFunc) Connect() (ZkConn, <-chan zk.Event, error) {
	return z()
}

// Disco is a service discovery framework orchestrated via zookeeper
type Disco struct {
	zkConnCreator ZkConnCreator

	zkConn               ZkConn
	eventChan            <-chan zk.Event
	GUIDbytes            [16]byte
	publishAddress       string
	myAdvertisedServices map[string]ServiceInstance
	myEphemeralNodes     map[string][]byte
	shouldQuit           chan struct{}
	eventLoopDone        chan struct{}
	ninjaMode            bool

	watchedMutex    sync.Mutex
	watchedServices map[string]*Service

	manualEvents chan zk.Event
	stateLog     log.Logger

	jsonMarshal func(v interface{}) ([]byte, error)
}

// BuilderConnector satisfies the disco zk connect interface for a zkplus.Builder
func BuilderConnector(b *zkplus.Builder) ZkConnCreator {
	return ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return b.BuildDirect()
	})
}

// Config controls optional parameters for disco
type Config struct {
	RandomSource io.Reader
	Logger       log.Logger
}

// DefaultConfig is used if any config values are nil
var DefaultConfig = &Config{
	RandomSource: rand.Reader,
	Logger:       log.DefaultLogger.CreateChild(),
}

// New creates a disco discovery/publishing service
func New(zkConnCreator ZkConnCreator, publishAddress string, config *Config) (*Disco, error) {
	conf := pointer.FillDefaultFrom(config, DefaultConfig).(*Config)
	var GUID [16]byte
	_, err := io.ReadFull(conf.RandomSource, GUID[:16])
	if err != nil {
		return nil, errors.Annotate(err, "cannot create random GUID")
	}

	d := &Disco{
		zkConnCreator:        zkConnCreator,
		myAdvertisedServices: make(map[string]ServiceInstance),
		myEphemeralNodes:     make(map[string][]byte),
		GUIDbytes:            GUID,
		jsonMarshal:          json.Marshal,
		publishAddress:       publishAddress,
		shouldQuit:           make(chan struct{}),
		eventLoopDone:        make(chan struct{}),
		watchedServices:      make(map[string]*Service),
		manualEvents:         make(chan zk.Event),
	}
	d.stateLog = log.NewContext(conf.Logger).With(logkey.GUID, d.GUID())
	d.zkConn, d.eventChan, err = zkConnCreator.Connect()
	if err != nil {
		return nil, errors.Annotate(err, "cannot create first zk connection")
	}
	go d.eventLoop()
	return d, nil
}

// NinjaMode will have future Advertise() calls no-op.  This is useful when
// connecting an application to a production or testing tier and not wanting to advertise yourself
// for incomming connections,
func (d *Disco) NinjaMode(ninjaMode bool) {
	d.ninjaMode = ninjaMode
}

func isServiceModificationEvent(eventType zk.EventType) bool {
	return eventType == zk.EventNodeDataChanged || eventType == zk.EventNodeDeleted || eventType == zk.EventNodeCreated || eventType == zk.EventNodeChildrenChanged
}

func (d *Disco) eventLoop() {
	defer close(d.eventLoopDone)
	for {
		select {
		case <-d.shouldQuit:
			return
		case e := <-d.manualEvents:
			log.IfErr(d.stateLog, d.processZkEvent(&e))
		case e := <-d.eventChan:
			log.IfErr(d.stateLog, d.processZkEvent(&e))
		}
	}
}

var errServiceDoesNotExist = errors.New("could not find service to refresh")

func (d *Disco) logInfoState(e *zk.Event) bool {
	if e.State == zk.StateDisconnected {
		d.stateLog.Log(logkey.ZkEvent, e, "Disconnected from zookeeper.  Will attempt to remake connection.")
		return true
	}
	if e.State == zk.StateConnecting {
		d.stateLog.Log(logkey.ZkEvent, e, "Server is now attempting to reconnect.")
		return true
	}
	return false
}

func (d *Disco) processZkEvent(e *zk.Event) error {
	d.stateLog.Log(logkey.ZkEvent, e, "disco event seen")
	d.logInfoState(e)
	if e.State == zk.StateHasSession {
		return d.refreshAll()
	}
	serviceName := ""
	if isServiceModificationEvent(e.Type) {
		// serviceName is in () /(___)/___
		serviceName = e.Path
		if serviceName[0] == '/' {
			serviceName = serviceName[1:]
		}
		parts := strings.SplitN(serviceName, "/", 2)
		serviceName = parts[0]
	}
	if serviceName != "" {
		d.stateLog.Log(logkey.DiscoService, serviceName, "refresh on service")
		service, err := func() (*Service, error) {
			d.watchedMutex.Lock()
			defer d.watchedMutex.Unlock()
			s, exist := d.watchedServices[serviceName]
			if exist {
				return s, nil
			}
			return nil, errServiceDoesNotExist
		}()
		if err != nil {
			d.stateLog.Log(logkey.ZkEvent, e, logkey.DiscoService, serviceName, log.Err, err, "Unable to find parent")
		} else {
			d.stateLog.Log(logkey.DiscoService, service, "refreshing")
			return service.refresh(d.zkConn)
		}
	}
	return nil
}

// Close any open disco connections making this disco unreliable for future updates
// TODO(jack): Close should also delete advertised services
func (d *Disco) Close() {
	close(d.shouldQuit)
	<-d.eventLoopDone
	d.zkConn.Close()
}

func (d *Disco) servicePath(serviceName string) string {
	return fmt.Sprintf("/%s/%s", serviceName, d.GUID())
}

// GUID that this disco advertises itself as
func (d *Disco) GUID() string {
	return fmt.Sprintf("%x", d.GUIDbytes)
}

func (d *Disco) myServiceData(serviceName string, payload interface{}, port uint16) ServiceInstance {
	return ServiceInstance{
		ID:                  d.GUID(),
		Name:                serviceName,
		Payload:             payload,
		Address:             d.publishAddress,
		Port:                port,
		RegistrationTimeUTC: time.Now().UnixNano() / int64(time.Millisecond),
		SslPort:             nil,
		ServiceType:         "DYNAMIC",
		URISpec:             nil,
	}
}

func (d *Disco) refreshAll() error {
	d.stateLog.Log("Refreshing all zk services")
	d.watchedMutex.Lock()
	defer d.watchedMutex.Unlock()
	for serviceName, serviceInstance := range d.myAdvertisedServices {
		l := log.NewContext(d.stateLog).With(logkey.DiscoService, serviceName)
		l.Log("refresh for service")
		log.IfErr(l, d.advertiseInZK(false, serviceName, serviceInstance))
	}
	for path, payload := range d.myEphemeralNodes {
		l := log.NewContext(d.stateLog).With(logkey.DiscoService, path)
		l.Log("refresh for node")
		log.IfErr(l, d.createInZk(false, path, payload))
	}
	for _, service := range d.watchedServices {
		log.IfErr(d.stateLog, service.refresh(d.zkConn))
	}
	return nil
}

func (d *Disco) createInZk(deleteIfExists bool, pathDirectory string, instanceBytes []byte) error {
	servicePath := d.servicePath(pathDirectory)
	exists, stat, _, err := d.zkConn.ExistsW(servicePath)
	if err != nil {
		return errors.Annotatef(err, "cannot ExistsW %s", servicePath)
	}
	if !deleteIfExists && exists {
		d.stateLog.Log("Service already exists.  Will not delete it")
		return nil
	}
	if exists {
		d.stateLog.Log(logkey.ZkPath, servicePath, "Clearing out old service path")
		// clear out the old version
		if err = d.zkConn.Delete(servicePath, stat.Version); err != nil {
			return errors.Annotatef(err, "unhandled ZK Delete of %s", servicePath)
		}
	}
	_, err = d.zkConn.Create(servicePath, instanceBytes, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	return err
}

func (d *Disco) advertiseInZK(deleteIfExists bool, serviceName string, instanceData ServiceInstance) error {
	// Need to create service root node
	_, err := d.zkConn.Create(fmt.Sprintf("/%s", serviceName), []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && errors.Tail(err) != zk.ErrNodeExists {
		return errors.Annotatef(err, "unhandled ZK error on Create of %s", serviceName)
	}
	instanceBytes, err := d.jsonMarshal(instanceData)
	if err != nil {
		return errors.Annotatef(err, "cannot JSON marshal data %v", instanceData)
	}
	if err = d.createInZk(deleteIfExists, serviceName, instanceBytes); err != nil {
		return errors.Annotatef(err, "cannot create node to advertise myself in disco")
	}
	return nil
}

// ErrDuplicateAdvertise is returned by Advertise if users try to Advertise the same service name
// twice
var ErrDuplicateAdvertise = errors.New("service name already advertised")

func (d *Disco) innerPreable(path string, err *error) func() {
	if d.ninjaMode {
		d.stateLog.Log(logkey.DiscoService, path, "Not advertising because disco is in ninja mode")
		return nil
	}
	return func() {
		d.stateLog.Log(log.Err, *err, "advertised result")
		if *err == nil {
			d.manualEvents <- zk.Event{
				Type:  zk.EventNodeChildrenChanged,
				State: zk.StateConnected,
				Path:  path,
			}
		}
	}
}

// Advertise yourself as hosting a service
func (d *Disco) Advertise(serviceName string, payload interface{}, port uint16) (err error) {
	f := d.innerPreable(serviceName, &err)
	if f == nil {
		return nil
	}
	// Note: Important to defer after we release the mutex since the chan send could be a blocking
	//       operation
	defer f()
	d.watchedMutex.Lock()
	defer d.watchedMutex.Unlock()
	d.stateLog.Log(logkey.DiscoService, serviceName, "Advertising myself on a service")
	_, exists := d.myAdvertisedServices[serviceName]
	if exists {
		return ErrDuplicateAdvertise
	}
	service := d.myServiceData(serviceName, payload, port)
	if err := d.advertiseInZK(true, serviceName, service); err != nil {
		return errors.Annotatef(err, "cannot advertise %s in disco", serviceName)
	}
	d.myAdvertisedServices[serviceName] = service
	return nil
}

// CreatePersistentEphemeralNode creates a persistent ephemeral node
func (d *Disco) CreatePersistentEphemeralNode(path string, payload []byte) (err error) {
	f := d.innerPreable(path, &err)
	if f == nil {
		return nil
	}
	// Note: Important to defer after we release the mutex since the chan send could be a blocking
	//       operation
	defer f()
	d.watchedMutex.Lock()
	defer d.watchedMutex.Unlock()
	_, err = d.zkConn.Create(fmt.Sprintf("/%s", path), []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && errors.Tail(err) != zk.ErrNodeExists {
		return errors.Annotatef(err, "unhandled ZK error on Create of %s", path)
	}
	if err = d.createInZk(true, path, payload); err != nil {
		return errors.Annotatef(err, "cannot create persistent ephemeral node")
	}
	d.myEphemeralNodes[path] = payload
	return nil
}

// Var returns an expvar variable that shows all the current disco services and the current
// list of endpoints seen for each service
func (d *Disco) Var() expvar.Var {
	return expvar.Func(func() interface{} {
		d.watchedMutex.Lock()
		defer d.watchedMutex.Unlock()
		ret := make(map[string][]ServiceInstance)
		for serviceName, service := range d.watchedServices {
			ret[serviceName] = service.ServiceInstances()
		}
		return ret
	})
}

// Services advertising for serviceName
func (d *Disco) Services(serviceName string) (*Service, error) {
	d.watchedMutex.Lock()
	defer d.watchedMutex.Unlock()
	s, exist := d.watchedServices[serviceName]
	if exist {
		return s, nil
	}
	ret := &Service{
		name:     serviceName,
		stateLog: log.NewContext(d.stateLog).With(logkey.DiscoService, serviceName),
	}
	ret.services.Store([]ServiceInstance{})
	refreshRes := ret.refresh(d.zkConn)
	if refreshRes == nil {
		d.watchedServices[serviceName] = ret
		return ret, nil
	}
	return nil, errors.Annotatef(refreshRes, "cannot refresh service %s", serviceName)
}

// ServiceInstances that represent instances of this service in your system
func (s *Service) ServiceInstances() []ServiceInstance {
	return s.services.Load().([]ServiceInstance)
}

// ForceInstances overrides a disco service to have exactly the passed instances forever.  Useful
// for debugging.
func (s *Service) ForceInstances(instances []ServiceInstance) {
	atomic.StoreInt64(&s.ignoreUpdates, 1)
	s.watchLock.Lock()
	defer s.watchLock.Unlock()
	s.services.Store(instances)
	for _, watch := range s.watches {
		watch()
	}
}

// Watch for changes to the members of this service
func (s *Service) Watch(watch ChangeWatch) {
	s.watchLock.Lock()
	defer s.watchLock.Unlock()
	s.watches = append(s.watches, watch)
}

func (s *Service) String() string {
	s.watchLock.Lock()
	defer s.watchLock.Unlock()
	return fmt.Sprintf("name=%s|len(watch)=%d|instances=%v", s.name, len(s.watches), s.services.Load())
}

func (s *Service) byteHashes() string {
	all := []string{}
	for _, i := range s.ServiceInstances() {
		all = append(all, string(i.uniqueHash()))
	}
	slice := sort.StringSlice(all)
	slice.Sort()
	r := ""
	for _, s := range all {
		r += s
	}
	return r
}

func childrenServices(logger log.Logger, serviceName string, children []string, zkConn ZkConn) ([]ServiceInstance, error) {
	logger.Log("getting services")
	ret := make([]ServiceInstance, len(children))

	allErrors := make([]error, 0, len(children))
	var wg sync.WaitGroup
	for index, child := range children {
		wg.Add(1)
		go func(child string, instanceAddr *ServiceInstance) {
			defer wg.Done()
			var bytes []byte
			var err2 error
			bytes, _, _, err2 = zkConn.GetW(fmt.Sprintf("/%s/%s", serviceName, child))
			if err2 != nil {
				allErrors = append(allErrors, errors.Annotatef(err2, "fail to GetW %s/%s", serviceName, child))
				return
			}
			err2 = json.Unmarshal(bytes, instanceAddr)
			if err2 != nil {
				allErrors = append(allErrors, errors.Annotatef(err2, "cannot unmarshal %s from %v", child, bytes))
				return
			}
		}(child, &ret[index]) // <--- Important b/c inside range
	}
	wg.Wait()
	return ret, errors.NewMultiErr(allErrors)
}

func (s *Service) refresh(zkConn ZkConn) error {
	if atomic.LoadInt64(&s.ignoreUpdates) != 0 {
		s.stateLog.Log("refresh flag set.  Ignoring refresh")
		return nil
	}
	s.stateLog.Log("refresh called")
	oldHash := s.byteHashes()
	children, _, _, err := zkConn.ChildrenW(fmt.Sprintf("/%s", s.name))
	if err != nil && errors.Tail(err) != zk.ErrNoNode {
		s.stateLog.Log(log.Err, err, "Error getting children")
		return errors.Annotatef(err, "unexpected zk error on childrenw")
	}

	if errors.Tail(err) == zk.ErrNoNode {
		exists, _, _, err := zkConn.ExistsW(fmt.Sprintf("/%s", s.name))
		if exists || err != nil {
			s.stateLog.Log(log.Err, err, "Unable to register exists watch!")
		}
		s.services.Store(make([]ServiceInstance, 0))
	} else {
		services, err := childrenServices(s.stateLog, s.name, children, zkConn)
		if err != nil {
			return err
		}
		s.services.Store(services)
	}
	s.watchLock.Lock()
	defer s.watchLock.Unlock()
	newHash := s.byteHashes()
	if oldHash != newHash {
		for _, w := range s.watches {
			w()
		}
	}
	return nil
}
