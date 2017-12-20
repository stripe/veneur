package clientcfg

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"context"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/distconf"
	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/logkey"
	"github.com/signalfx/golib/sfxclient"
	"github.com/signalfx/golib/timekeeper"
)

// ClientConfig configures a SfxClient
type ClientConfig struct {
	SourceName        *distconf.Str
	AuthToken         *distconf.Str
	Endpoint          *distconf.Str
	ReportingInterval *distconf.Duration
	TimeKeeper        timekeeper.TimeKeeper
	OsHostname        func() (name string, err error)
}

// Load the client config values from distconf
func (c *ClientConfig) Load(d *distconf.Distconf) {
	c.SourceName = d.Str("signalfuse.sourceName", "")
	c.AuthToken = d.Str("sf.metrics.auth_token", "")
	c.Endpoint = d.Str("sf.metrics.statsendpoint", "")
	c.ReportingInterval = d.Duration("sf.metrics.report_interval", time.Second)
	c.TimeKeeper = timekeeper.RealTime{}
	c.OsHostname = os.Hostname
}

// DefaultDimensions extracts default sfxclient dimensions that identify the host
func DefaultDimensions(conf *ClientConfig) (map[string]string, error) {
	signalfuseSourceName := conf.SourceName.Get()
	if signalfuseSourceName != "" {
		return map[string]string{"sf_source": signalfuseSourceName}, nil
	}
	hostname, err := conf.OsHostname()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get hostname for default source")
	}
	return map[string]string{"sf_source": hostname}, nil
}

// ClientConfigChangerSink is a sink that can update auth and endpoint values
type ClientConfigChangerSink struct {
	Destination *sfxclient.HTTPSink
	mu          sync.RWMutex
	logger      log.Logger
	urlParse    func(string) (url *url.URL, err error)
}

// WatchSinkChanges returns a new ClientConfigChangerSink that wraps a sink with auth/endpoint changes from distconf
func WatchSinkChanges(sink sfxclient.Sink, Conf *ClientConfig, logger log.Logger) sfxclient.Sink {
	httpSink, ok := sink.(*sfxclient.HTTPSink)
	if !ok {
		return sink
	}
	ret := &ClientConfigChangerSink{
		Destination: httpSink,
		urlParse:    url.Parse,
		logger:      logger,
	}
	ret.authTokenWatch(Conf.AuthToken, "")
	ret.endpointWatch(Conf.Endpoint, "")
	ret.endpointWatch(Conf.Endpoint, "")
	Conf.Endpoint.Watch(ret.endpointWatch)
	Conf.AuthToken.Watch(ret.authTokenWatch)
	return ret
}

// AddDatapoints forwards the call to Destination
func (s *ClientConfigChangerSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Destination.AddDatapoints(ctx, points)
}

func (s *ClientConfigChangerSink) authTokenWatch(str *distconf.Str, oldValue string) {
	s.logger.Log("auth watch")
	s.mu.Lock()
	s.Destination.AuthToken = str.Get()
	s.mu.Unlock()
}

// endpointWatch returns a distconf watch that sets the correct ingest endpoint for a signalfx
// client
func (s *ClientConfigChangerSink) endpointWatch(str *distconf.Str, oldValue string) {
	s.logger.Log("endpoint watch")
	e := str.Get()
	if e == "" {
		return
	}
	if !strings.HasPrefix(e, "http") {
		e = "http://" + e
	}
	u, err := s.urlParse(e)
	if err != nil {
		s.logger.Log(logkey.Endpoint, e, log.Err, err, "unable to parse URL")
		return
	}
	if u.Path == "" {
		u.Path = "/v2/datapoint"
	}
	if e != "" {
		s.logger.Log(logkey.Endpoint, e, logkey.URL, u, "updating reporter endpoint")
		s.mu.Lock()
		s.Destination.DatapointEndpoint = u.String()
		s.mu.Unlock()
	}
}
