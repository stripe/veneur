package splunk

import (
	"context"
	"io"
	"net/http"
	"net/url"

	uuid "github.com/satori/go.uuid"
)

type hecClient struct {
	token     string
	serverURL *url.URL
	idGen     uuid.UUID
}

func newHecClient(serverURL string, token string) (*hecClient, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	cl := hecClient{token: token, serverURL: u, idGen: id}
	return &cl, nil
}

const rawEndpointStr = "services/collector"

var rawEndpoint *url.URL

func init() {
	var err error
	rawEndpoint, err = url.Parse(rawEndpointStr)
	if err != nil {
		panic(err)
	}
}

// newRequest creates a new streaming HEC raw request and returns the
// writer to it. The request is submitted when the writer is closed.
func (c *hecClient) newRequest() *hecRequest {
	req := &hecRequest{url: c.url(c.idGen.String()), authHeader: c.authHeader()}
	req.r, req.w = io.Pipe()
	return req
}

type hecRequest struct {
	r          io.ReadCloser
	w          io.WriteCloser
	url        string
	authHeader string
}

func (r *hecRequest) Start(ctx context.Context) (*http.Request, io.Writer, error) {
	req, err := http.NewRequest("POST", r.url, r.r)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Add("Authorization", r.authHeader)
	req = req.WithContext(ctx)

	return req, r.w, nil
}

func (r *hecRequest) Close() error {
	return r.w.Close()
}

func (c *hecClient) url(channel string) string {
	endpoint := c.serverURL.ResolveReference(rawEndpoint)
	q := endpoint.Query()
	q.Add("channel", channel)
	endpoint.RawQuery = q.Encode()
	return endpoint.String()
}

func (c *hecClient) authHeader() string {
	return "Splunk " + c.token
}

// Response represents the JSON-parseable response from a splunk HEC
// server.
type Response struct {
	Text               string `json:"text,omitempty"`
	Code               int    `json:"code"`
	InvalidEventNumber *int   `json:"invalid-event-number,omitempty"`
}
