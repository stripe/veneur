package disco

import (
	"math/rand"
	"net"

	"time"

	"github.com/signalfx/golib/errors"

	"git.apache.org/thrift.git/lib/go/thrift"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/logkey"
)

// ThriftTransport can be used as the transport layer for thrift, connecting to services discovered
// by disco
type ThriftTransport struct {
	currentTransport thrift.TTransport
	WrappedFactory   thrift.TTransportFactory
	service          *Service
	randSource       *rand.Rand
	Dialer           net.Dialer
	logger           log.Logger
}

var _ thrift.TTransport = &ThriftTransport{}

// NewThriftTransport creates a new ThriftTransport.  The default transport factory is TFramed
func NewThriftTransport(service *Service, timeout time.Duration) *ThriftTransport {
	return NewThriftTransportWithMaxBufferSize(service, timeout, thrift.DEFAULT_MAX_LENGTH)
}

// NewThriftTransportWithMaxBufferSize creates a new ThriftTransport for TFramedTransport but sets
// the maximum size for request frames.
func NewThriftTransportWithMaxBufferSize(service *Service, timeout time.Duration, maxLength uint32) *ThriftTransport {
	return &ThriftTransport{
		WrappedFactory: thrift.NewTFramedTransportFactoryMaxLength(thrift.NewTTransportFactory(), maxLength),
		service:        service,
		randSource:     rand.New(rand.NewSource(time.Now().UnixNano())),
		Dialer: net.Dialer{
			Timeout: timeout,
		},
		logger: log.NewContext(service.stateLog).With(logkey.Protocol, "thrift"),
	}
}

// Flush the underline transport
func (d *ThriftTransport) Flush() (err error) {
	if d.currentTransport == nil {
		return nil
	}
	err = d.currentTransport.Flush()
	if err != nil {
		log.IfErr(d.logger, d.currentTransport.Close())
		d.currentTransport = nil
	}
	return errors.Annotate(err, "cannot flush current transport")
}

// IsOpen will return true if there is a connected underline transport and it is open
func (d *ThriftTransport) IsOpen() bool {
	return d.currentTransport != nil && d.currentTransport.IsOpen()
}

// Close and nil the underline transport
func (d *ThriftTransport) Close() error {
	if d.currentTransport == nil {
		return nil
	}
	ret := d.currentTransport.Close()
	d.currentTransport = nil
	return errors.Annotate(ret, "cannot close current transport")
}

// Read bytes from underline transport if it is not nil.  Exact definition defined in TTransport
func (d *ThriftTransport) Read(b []byte) (n int, err error) {
	if d.currentTransport == nil {
		return 0, thrift.NewTTransportException(thrift.NOT_OPEN, "")
	}
	var e1 error
	var e2 error
	n, e1 = d.currentTransport.Read(b)
	if e1 != nil {
		e2 = d.Close()
	}
	err = errors.NewMultiErr([]error{e1, e2})
	return
}

// Write bytes to underline transport if it is not nil.  Exact definition defined in TTransport
func (d *ThriftTransport) Write(b []byte) (n int, err error) {
	if d.currentTransport == nil {
		return 0, thrift.NewTTransportException(thrift.NOT_OPEN, "")
	}
	var e1 error
	var e2 error
	n, e1 = d.currentTransport.Write(b)
	if e1 != nil {
		e2 = d.Close()
	}
	err = errors.NewMultiErr([]error{e1, e2})
	return
}

// ErrNoInstance is returned by NextConnection if the service has no instances
var ErrNoInstance = errors.New("no thrift instances in disco")

// ErrNoInstanceOpen is returned by NextConnection if it cannot connect to any service ports
var ErrNoInstanceOpen = errors.New("no thrift instances is open")

// NextConnection will connect this transport to another random disco service, or return an error
// if no disco service can be dialed
func (d *ThriftTransport) NextConnection() error {
	if d.currentTransport != nil && d.currentTransport.IsOpen() {
		// TODO: Log errors on Close
		log.IfErr(d.logger, d.currentTransport.Close())
	}
	instances := d.service.ServiceInstances()
	if len(instances) == 0 {
		return ErrNoInstance
	}
	startIndex := d.randSource.Intn(len(instances))
	for i := 0; i < len(instances); i++ {
		instance := &instances[(startIndex+i)%len(instances)]
		conn, err := d.Dialer.Dial("tcp", instance.DialString())
		if err != nil {
			d.logger.Log(log.Err, err, "Unable to dial instance")
			continue
		}
		d.currentTransport = d.WrappedFactory.GetTransport(thrift.NewTSocketFromConnTimeout(conn, d.Dialer.Timeout))
		return nil
	}
	d.currentTransport = nil
	return ErrNoInstanceOpen
}

// Open a connection if one does not exist, otherwise do nothing.
func (d *ThriftTransport) Open() error {
	if d.currentTransport == nil || !d.currentTransport.IsOpen() {
		return errors.Annotate(d.NextConnection(), "cannot open next connection")
	}
	return nil
}

// RemainingBytes of underline transport, or maxSize if not set
func (d *ThriftTransport) RemainingBytes() uint64 {
	if d.currentTransport == nil {
		// Looking at the thrift code, I think this is the right thing to do.  This public interface
		// and function is not documented by thrift.
		const maxSize = ^uint64(0)
		return maxSize // the thruth is, we just don't know unless framed is used
	}
	return d.currentTransport.RemainingBytes()
}
