package envoyhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeInner struct {
	lastReq *http.Request
}

func (t *fakeInner) RoundTrip(r *http.Request) (*http.Response, error) {
	t.lastReq = r
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
	}, nil
}

func TestUsesInner(t *testing.T) {
	fi := fakeInner{}
	trns := Transport{Inner: &fi}

	req, _ := http.NewRequest("POST", "http://example.com", nil)
	cl := &http.Client{Transport: &trns}

	resp, err := cl.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, fi.lastReq, req)
}

func TestHeaders(t *testing.T) {
	trns := Transport{}
	cl := &http.Client{Transport: &trns}

	const (
		retries = 17
		perTry  = 23 * time.Millisecond
		overall = 19 * time.Second
	)

	deadline := time.Now().Add(overall)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, strconv.Itoa(retries), r.Header.Get("x-envoy-max-retries"))
		assert.Equal(t, strconv.Itoa(int(perTry/time.Millisecond)),
			r.Header.Get("x-envoy-upstream-rq-per-try-timeout-ms"))

		overallFromHeaders, err := strconv.Atoi(r.Header.Get("x-envoy-upstream-rq-timeout-ms"))
		require.NoError(t, err)
		deadlineFromHeaders := time.Now().Add(time.Duration(overallFromHeaders) * time.Millisecond)
		assert.WithinDuration(t, deadline, deadlineFromHeaders, time.Second)

		w.Write([]byte(`ok`))
	}))

	ctx := context.Background()
	req, _ := http.NewRequest("POST", srv.URL, nil)

	// Set the values:
	ctx = WithMaxRetries(ctx, retries)
	ctx = WithPerRetryTimeout(ctx, perTry)
	var cancel func()
	ctx, cancel = context.WithDeadline(ctx, deadline)
	defer cancel()
	req = req.WithContext(ctx)

	// Make the request:
	resp, err := cl.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestAllowance(t *testing.T) {
	trns := Transport{ExtraAllowance: 5 * time.Second}
	cl := &http.Client{Transport: &trns}

	const (
		retries = 17
		perTry  = 23 * time.Millisecond
		overall = 19 * time.Second
	)

	deadline := time.Now().Add(overall)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		overallFromHeaders, err := strconv.Atoi(r.Header.Get("x-envoy-upstream-rq-timeout-ms"))
		require.NoError(t, err)
		deadlineFromHeaders := time.Now().Add(time.Duration(overallFromHeaders) * time.Millisecond)
		assert.WithinDuration(t, deadline.Add(-5*time.Second), deadlineFromHeaders, time.Second)
		w.Write([]byte(`ok`))
	}))

	ctx := context.Background()
	req, _ := http.NewRequest("POST", srv.URL, nil)

	// Set the values:
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	req = req.WithContext(ctx)

	// Make the request:
	resp, err := cl.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestLeaveWellEnoughAlone(t *testing.T) {
	trns := Transport{}
	cl := &http.Client{Transport: &trns}

	const (
		retries = 17
		perTry  = 23 * time.Millisecond
		overall = 19 * time.Second
	)

	deadline := time.Now().Add(overall)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "oink", r.Header.Get("x-envoy-max-retries"))
		assert.Equal(t, "blug", r.Header.Get("x-envoy-upstream-rq-per-try-timeout-ms"))
		assert.Equal(t, "glub", r.Header.Get("x-envoy-upstream-rq-timeout-ms"))
		w.Write([]byte(`ok`))
	}))

	ctx := context.Background()
	req, _ := http.NewRequest("POST", srv.URL, nil)
	headers := req.Header
	headers.Add("x-envoy-max-retries", "oink")
	headers.Add("x-envoy-upstream-rq-per-try-timeout-ms", "blug")
	headers.Add("x-envoy-upstream-rq-timeout-ms", "glub")
	req.Header = headers

	// Set the values:
	ctx = WithMaxRetries(ctx, retries)
	ctx = WithPerRetryTimeout(ctx, perTry)
	var cancel func()
	ctx, cancel = context.WithDeadline(ctx, deadline)
	defer cancel()
	req = req.WithContext(ctx)

	// Make the request:
	resp, err := cl.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
