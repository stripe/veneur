package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// Transport, like http.Transport, implements http.RoundTripper. Set an object
// of type Transport as the Transport field on an http.Client to send requests
// via the proxy.
type Transport struct {
	shadow http.Transport
}

// New returns a Transport that uses the UNIX domain socket at the given path.
// Set the Transport field of an http.Client to send requests via the proxy.
// checkResponse is a function that will be called on each response and can
// be used for filtering or additional processing.
func NewUnixTransport(path string) *Transport {
	dial := func(network, addr string) (net.Conn, error) {
		return net.Dial("unix", path)
	}
	shadow := http.Transport{
		Dial:                  dial,
		DialTLS:               dial,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}
	return &Transport{shadow}
}

// RoundTrip implements the http.RoundTripper interface.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := *req
	url2 := *req.URL
	req2.URL = &url2
	req2.URL.Opaque = fmt.Sprintf("//%s%s", req.URL.Host, req.URL.EscapedPath())
	resp, err := t.shadow.RoundTrip(&req2)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
