// Copyright 2013 Ooyala, Inc.

/*
Package statsd provides a Go dogstatsd client. Dogstatsd extends the popular statsd,
adding tags and histograms and pushing upstream to Datadog.

Refer to http://docs.datadoghq.com/guides/dogstatsd/ for information about DogStatsD.

Example Usage:

    // Create the client
    c, err := statsd.New("127.0.0.1:8125")
    if err != nil {
        log.Fatal(err)
    }
    // Prefix every metric with the app name
    c.Namespace = "flubber."
    // Send the EC2 availability zone as a tag with every metric
    c.Tags = append(c.Tags, "us-east-1a")
    err = c.Gauge("request.duration", 1.2, nil, 1)

statsd is based on go-statsd-client.
*/
package statsd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

/*
OptimalUDPPayloadSize defines the optimal payload size for a UDP datagram, 1432 bytes
is optimal for regular networks with an MTU of 1500 so datagrams don't get
fragmented. It's generally recommended not to fragment UDP datagrams as losing
a single fragment will cause the entire datagram to be lost.
*/
const OptimalUDPPayloadSize = 1432

/*
MaxUDPPayloadSize defines the maximum payload size for a UDP datagram.
Its value comes from the calculation: 65535 bytes Max UDP datagram size -
8byte UDP header - 60byte max IP headers
any number greater than that will see frames being cut out.
*/
const MaxUDPPayloadSize = 65467

// DefaultUDPBufferPoolSize is the default size of the buffer pool for UDP clients.
const DefaultUDPBufferPoolSize = 2048

// DefaultUDSBufferPoolSize is the default size of the buffer pool for UDS clients.
const DefaultUDSBufferPoolSize = 512

/*
DefaultMaxAgentPayloadSize is the default maximum payload size the agent
can receive. This can be adjusted by changing dogstatsd_buffer_size in the
agent configuration file datadog.yaml.
*/
const DefaultMaxAgentPayloadSize = 8192

/*
TelemetryInterval is the interval at which telemetry will be sent by the client.
*/
const TelemetryInterval = 10 * time.Second

/*
clientTelemetryTag is a tag identifying this specific client.
*/
var clientTelemetryTag = "client:go"

/*
clientVersionTelemetryTag is a tag identifying this specific client version.
*/
var clientVersionTelemetryTag = "client_version:3.7.2"

/*
UnixAddressPrefix holds the prefix to use to enable Unix Domain Socket
traffic instead of UDP.
*/
const UnixAddressPrefix = "unix://"

/*
ddEnvTagsMapping is a mapping of each "DD_" prefixed environment variable
to a specific tag name.
*/
var ddEnvTagsMapping = map[string]string{
	// Client-side entity ID injection for container tagging.
	"DD_ENTITY_ID": "dd.internal.entity_id",
	// The name of the env in which the service runs.
	"DD_ENV": "env",
	// The name of the running service.
	"DD_SERVICE": "service",
	// The current version of the running service.
	"DD_VERSION": "version",
}

type metricType int

const (
	gauge metricType = iota
	count
	histogram
	distribution
	set
	timing
	event
	serviceCheck
)

type ReceivingMode int

const (
	MutexMode ReceivingMode = iota
	ChannelMode
)

type metric struct {
	metricType metricType
	namespace  string
	globalTags []string
	name       string
	fvalue     float64
	ivalue     int64
	svalue     string
	evalue     *Event
	scvalue    *ServiceCheck
	tags       []string
	rate       float64
}

type noClientErr string

// ErrNoClient is returned if statsd reporting methods are invoked on
// a nil client.
const ErrNoClient = noClientErr("statsd client is nil")

func (e noClientErr) Error() string {
	return string(e)
}

// ClientInterface is an interface that exposes the common client functions for the
// purpose of being able to provide a no-op client or even mocking. This can aid
// downstream users' with their testing.
type ClientInterface interface {
	// Gauge measures the value of a metric at a particular time.
	Gauge(name string, value float64, tags []string, rate float64) error

	// Count tracks how many times something happened per second.
	Count(name string, value int64, tags []string, rate float64) error

	// Histogram tracks the statistical distribution of a set of values on each host.
	Histogram(name string, value float64, tags []string, rate float64) error

	// Distribution tracks the statistical distribution of a set of values across your infrastructure.
	Distribution(name string, value float64, tags []string, rate float64) error

	// Decr is just Count of -1
	Decr(name string, tags []string, rate float64) error

	// Incr is just Count of 1
	Incr(name string, tags []string, rate float64) error

	// Set counts the number of unique elements in a group.
	Set(name string, value string, tags []string, rate float64) error

	// Timing sends timing information, it is an alias for TimeInMilliseconds
	Timing(name string, value time.Duration, tags []string, rate float64) error

	// TimeInMilliseconds sends timing information in milliseconds.
	// It is flushed by statsd with percentiles, mean and other info (https://github.com/etsy/statsd/blob/master/docs/metric_types.md#timing)
	TimeInMilliseconds(name string, value float64, tags []string, rate float64) error

	// Event sends the provided Event.
	Event(e *Event) error

	// SimpleEvent sends an event with the provided title and text.
	SimpleEvent(title, text string) error

	// ServiceCheck sends the provided ServiceCheck.
	ServiceCheck(sc *ServiceCheck) error

	// SimpleServiceCheck sends an serviceCheck with the provided name and status.
	SimpleServiceCheck(name string, status ServiceCheckStatus) error

	// Close the client connection.
	Close() error

	// Flush forces a flush of all the queued dogstatsd payloads.
	Flush() error

	// SetWriteTimeout allows the user to set a custom write timeout.
	SetWriteTimeout(d time.Duration) error
}

// A Client is a handle for sending messages to dogstatsd.  It is safe to
// use one Client from multiple goroutines simultaneously.
type Client struct {
	// Sender handles the underlying networking protocol
	sender *sender
	// Namespace to prepend to all statsd calls
	Namespace string
	// Tags are global tags to be added to every statsd call
	Tags []string
	// skipErrors turns off error passing and allows UDS to emulate UDP behaviour
	SkipErrors    bool
	flushTime     time.Duration
	bufferPool    *bufferPool
	buffer        *statsdBuffer
	metrics       *ClientMetrics
	telemetryTags []string
	stop          chan struct{}
	wg            sync.WaitGroup
	bufferShards  []*worker
	closerLock    sync.Mutex
	receiveMode   ReceivingMode
	agg           *aggregator
	options       []Option
	addrOption    string
}

// ClientMetrics contains metrics about the client
type ClientMetrics struct {
	TotalMetrics          uint64
	TotalEvents           uint64
	TotalServiceChecks    uint64
	TotalDroppedOnReceive uint64
}

// Verify that Client implements the ClientInterface.
// https://golang.org/doc/faq#guarantee_satisfies_interface
var _ ClientInterface = &Client{}

// New returns a pointer to a new Client given an addr in the format "hostname:port" or
// "unix:///path/to/socket".
func New(addr string, options ...Option) (*Client, error) {
	var w statsdWriter
	o, err := resolveOptions(options)
	if err != nil {
		return nil, err
	}

	var writerType string
	optimalPayloadSize := OptimalUDPPayloadSize
	defaultBufferPoolSize := DefaultUDPBufferPoolSize
	if !strings.HasPrefix(addr, UnixAddressPrefix) {
		w, err = newUDPWriter(addr)
		writerType = "udp"
	} else {
		// FIXME: The agent has a performance pitfall preventing us from using better defaults here.
		// Once it's fixed, use `DefaultMaxAgentPayloadSize` and `DefaultUDSBufferPoolSize` instead.
		optimalPayloadSize = OptimalUDPPayloadSize
		defaultBufferPoolSize = DefaultUDPBufferPoolSize
		w, err = newUDSWriter(addr[len(UnixAddressPrefix):])
		writerType = "uds"
	}
	if err != nil {
		return nil, err
	}

	if o.MaxBytesPerPayload == 0 {
		o.MaxBytesPerPayload = optimalPayloadSize
	}
	if o.BufferPoolSize == 0 {
		o.BufferPoolSize = defaultBufferPoolSize
	}
	if o.SenderQueueSize == 0 {
		o.SenderQueueSize = defaultBufferPoolSize
	}
	client, err := newWithWriter(w, o, writerType)
	if err == nil {
		client.options = append(client.options, options...)
		client.addrOption = addr
	}
	return client, err
}

// NewWithWriter creates a new Client with given writer. Writer is a
// io.WriteCloser + SetWriteTimeout(time.Duration) error
func NewWithWriter(w statsdWriter, options ...Option) (*Client, error) {
	o, err := resolveOptions(options)
	if err != nil {
		return nil, err
	}
	return newWithWriter(w, o, "custom")
}

// CloneWithExtraOptions create a new Client with extra options
func CloneWithExtraOptions(c *Client, options ...Option) (*Client, error) {
	if c == nil {
		return nil, ErrNoClient
	}

	if c.addrOption == "" {
		return nil, fmt.Errorf("can't clone client with no addrOption")
	}
	opt := append(c.options, options...)
	return New(c.addrOption, opt...)
}

func newWithWriter(w statsdWriter, o *Options, writerName string) (*Client, error) {

	w.SetWriteTimeout(o.WriteTimeoutUDS)

	c := Client{
		Namespace: o.Namespace,
		Tags:      o.Tags,
		metrics:   &ClientMetrics{},
	}
	if o.Aggregation {
		c.agg = newAggregator(&c)
		c.agg.start(o.AggregationFlushInterval)
	}

	// Inject values of DD_* environment variables as global tags.
	for envName, tagName := range ddEnvTagsMapping {
		if value := os.Getenv(envName); value != "" {
			c.Tags = append(c.Tags, fmt.Sprintf("%s:%s", tagName, value))
		}
	}

	c.telemetryTags = append(c.Tags, clientTelemetryTag, clientVersionTelemetryTag, "client_transport:"+writerName)

	if o.MaxBytesPerPayload == 0 {
		o.MaxBytesPerPayload = OptimalUDPPayloadSize
	}
	if o.BufferPoolSize == 0 {
		o.BufferPoolSize = DefaultUDPBufferPoolSize
	}
	if o.SenderQueueSize == 0 {
		o.SenderQueueSize = DefaultUDPBufferPoolSize
	}

	c.receiveMode = o.ReceiveMode
	c.bufferPool = newBufferPool(o.BufferPoolSize, o.MaxBytesPerPayload, o.MaxMessagesPerPayload)
	c.buffer = c.bufferPool.borrowBuffer()
	c.sender = newSender(w, o.SenderQueueSize, c.bufferPool)
	for i := 0; i < o.BufferShardCount; i++ {
		w := newWorker(c.bufferPool, c.sender)
		c.bufferShards = append(c.bufferShards, w)
		if c.receiveMode == ChannelMode {
			w.startReceivingMetric(o.ChannelModeBufferSize) // TODO make it configurable
		}
	}
	c.flushTime = o.BufferFlushInterval
	c.stop = make(chan struct{}, 1)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.watch()
	}()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if o.Telemetry {
			c.telemetry()
		}
	}()
	return &c, nil
}

// NewBuffered returns a Client that buffers its output and sends it in chunks.
// Buflen is the length of the buffer in number of commands.
//
// When addr is empty, the client will default to a UDP client and use the DD_AGENT_HOST
// and (optionally) the DD_DOGSTATSD_PORT environment variables to build the target address.
func NewBuffered(addr string, buflen int) (*Client, error) {
	return New(addr, WithMaxMessagesPerPayload(buflen))
}

// SetWriteTimeout allows the user to set a custom UDS write timeout. Not supported for UDP.
func (c *Client) SetWriteTimeout(d time.Duration) error {
	if c == nil {
		return ErrNoClient
	}
	return c.sender.transport.SetWriteTimeout(d)
}

func (c *Client) watch() {
	ticker := time.NewTicker(c.flushTime)

	for {
		select {
		case <-ticker.C:
			for _, w := range c.bufferShards {
				w.flush()
			}
		case <-c.stop:
			ticker.Stop()
			return
		}
	}
}

func (c *Client) telemetry() {
	ticker := time.NewTicker(TelemetryInterval)
	for {
		select {
		case <-ticker.C:
			for _, m := range c.flushTelemetry() {
				c.send(m)
			}
		case <-c.stop:
			ticker.Stop()
			return
		}
	}
}

// flushTelemetry returns Telemetry metrics to be flushed. It's its own function to ease testing.
func (c *Client) flushTelemetry() []metric {
	m := []metric{}

	// same as Count but without global namespace
	telemetryCount := func(name string, value int64) {
		m = append(m, metric{metricType: count, name: name, ivalue: value, tags: c.telemetryTags, rate: 1})
	}

	clientMetrics := c.FlushTelemetryMetrics()
	telemetryCount("datadog.dogstatsd.client.metrics", int64(clientMetrics.TotalMetrics))
	telemetryCount("datadog.dogstatsd.client.events", int64(clientMetrics.TotalEvents))
	telemetryCount("datadog.dogstatsd.client.service_checks", int64(clientMetrics.TotalServiceChecks))
	telemetryCount("datadog.dogstatsd.client.metric_dropped_on_receive", int64(clientMetrics.TotalDroppedOnReceive))

	senderMetrics := c.sender.flushTelemetryMetrics()
	telemetryCount("datadog.dogstatsd.client.packets_sent", int64(senderMetrics.TotalSentPayloads))
	telemetryCount("datadog.dogstatsd.client.bytes_sent", int64(senderMetrics.TotalSentBytes))
	telemetryCount("datadog.dogstatsd.client.packets_dropped", int64(senderMetrics.TotalDroppedPayloads))
	telemetryCount("datadog.dogstatsd.client.bytes_dropped", int64(senderMetrics.TotalDroppedBytes))
	telemetryCount("datadog.dogstatsd.client.packets_dropped_queue", int64(senderMetrics.TotalDroppedPayloadsQueueFull))
	telemetryCount("datadog.dogstatsd.client.bytes_dropped_queue", int64(senderMetrics.TotalDroppedBytesQueueFull))
	telemetryCount("datadog.dogstatsd.client.packets_dropped_writer", int64(senderMetrics.TotalDroppedPayloadsWriter))
	telemetryCount("datadog.dogstatsd.client.bytes_dropped_writer", int64(senderMetrics.TotalDroppedBytesWriter))
	return m
}

// Flush forces a flush of all the queued dogstatsd payloads
// This method is blocking and will not return until everything is sent
// through the network
func (c *Client) Flush() error {
	if c == nil {
		return ErrNoClient
	}
	for _, w := range c.bufferShards {
		w.flush()
	}
	c.sender.flush()
	return nil
}

func (c *Client) FlushTelemetryMetrics() ClientMetrics {
	return ClientMetrics{
		TotalMetrics:          atomic.SwapUint64(&c.metrics.TotalMetrics, 0),
		TotalEvents:           atomic.SwapUint64(&c.metrics.TotalEvents, 0),
		TotalServiceChecks:    atomic.SwapUint64(&c.metrics.TotalServiceChecks, 0),
		TotalDroppedOnReceive: atomic.SwapUint64(&c.metrics.TotalDroppedOnReceive, 0),
	}
}

func (c *Client) send(m metric) error {
	if c == nil {
		return ErrNoClient
	}

	m.globalTags = c.Tags
	m.namespace = c.Namespace

	h := hashString32(m.name)
	worker := c.bufferShards[h%uint32(len(c.bufferShards))]

	if c.receiveMode == ChannelMode {
		select {
		case worker.inputMetrics <- m:
		default:
			atomic.AddUint64(&c.metrics.TotalDroppedOnReceive, 1)
		}
		return nil
	}
	return worker.processMetric(m)
}

// Gauge measures the value of a metric at a particular time.
func (c *Client) Gauge(name string, value float64, tags []string, rate float64) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalMetrics, 1)
	if c.agg != nil {
		return c.agg.gauge(name, value, tags, rate)
	}
	return c.send(metric{metricType: gauge, name: name, fvalue: value, tags: tags, rate: rate})
}

// Count tracks how many times something happened per second.
func (c *Client) Count(name string, value int64, tags []string, rate float64) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalMetrics, 1)
	if c.agg != nil {
		return c.agg.count(name, value, tags, rate)
	}
	return c.send(metric{metricType: count, name: name, ivalue: value, tags: tags, rate: rate})
}

// Histogram tracks the statistical distribution of a set of values on each host.
func (c *Client) Histogram(name string, value float64, tags []string, rate float64) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalMetrics, 1)
	return c.send(metric{metricType: histogram, name: name, fvalue: value, tags: tags, rate: rate})
}

// Distribution tracks the statistical distribution of a set of values across your infrastructure.
func (c *Client) Distribution(name string, value float64, tags []string, rate float64) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalMetrics, 1)
	return c.send(metric{metricType: distribution, name: name, fvalue: value, tags: tags, rate: rate})
}

// Decr is just Count of -1
func (c *Client) Decr(name string, tags []string, rate float64) error {
	return c.Count(name, -1, tags, rate)
}

// Incr is just Count of 1
func (c *Client) Incr(name string, tags []string, rate float64) error {
	return c.Count(name, 1, tags, rate)
}

// Set counts the number of unique elements in a group.
func (c *Client) Set(name string, value string, tags []string, rate float64) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalMetrics, 1)
	if c.agg != nil {
		return c.agg.set(name, value, tags, rate)
	}
	return c.send(metric{metricType: set, name: name, svalue: value, tags: tags, rate: rate})
}

// Timing sends timing information, it is an alias for TimeInMilliseconds
func (c *Client) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return c.TimeInMilliseconds(name, value.Seconds()*1000, tags, rate)
}

// TimeInMilliseconds sends timing information in milliseconds.
// It is flushed by statsd with percentiles, mean and other info (https://github.com/etsy/statsd/blob/master/docs/metric_types.md#timing)
func (c *Client) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalMetrics, 1)
	return c.send(metric{metricType: timing, name: name, fvalue: value, tags: tags, rate: rate})
}

// Event sends the provided Event.
func (c *Client) Event(e *Event) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalEvents, 1)
	return c.send(metric{metricType: event, evalue: e, rate: 1})
}

// SimpleEvent sends an event with the provided title and text.
func (c *Client) SimpleEvent(title, text string) error {
	e := NewEvent(title, text)
	return c.Event(e)
}

// ServiceCheck sends the provided ServiceCheck.
func (c *Client) ServiceCheck(sc *ServiceCheck) error {
	if c == nil {
		return ErrNoClient
	}
	atomic.AddUint64(&c.metrics.TotalServiceChecks, 1)
	return c.send(metric{metricType: serviceCheck, scvalue: sc, rate: 1})
}

// SimpleServiceCheck sends an serviceCheck with the provided name and status.
func (c *Client) SimpleServiceCheck(name string, status ServiceCheckStatus) error {
	sc := NewServiceCheck(name, status)
	return c.ServiceCheck(sc)
}

// Close the client connection.
func (c *Client) Close() error {
	if c == nil {
		return ErrNoClient
	}

	// Acquire closer lock to ensure only one thread can close the stop channel
	c.closerLock.Lock()
	defer c.closerLock.Unlock()

	// Notify all other threads that they should stop
	select {
	case <-c.stop:
		return nil
	default:
	}
	close(c.stop)

	if c.receiveMode == ChannelMode {
		for _, w := range c.bufferShards {
			w.stopReceivingMetric()
		}
	}

	// Wait for the threads to stop
	c.wg.Wait()

	// Finally flush any remaining metrics that may have come in at the last moment
	if c.agg != nil {
		c.agg.stop()
	}
	c.Flush()

	return c.sender.close()
}
