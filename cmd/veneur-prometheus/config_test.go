package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHTTPClientHTTP(t *testing.T) {
	client, err := newHTTPClient("", "", "", "")
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	res, err := client.Get(ts.URL)
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusOK)
}

func TestGetHTTPClientHTTPS(t *testing.T) {
	client, err := newHTTPClient("", "./testdata/client.pem", "./testdata/client.key", "./testdata/root.pem")
	require.NoError(t, err)

	caCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile("./testdata/root.pem")
	assert.NoError(t, err)
	caCertPool.AppendCertsFromPEM(caCert)

	serverCert, err := tls.LoadX509KeyPair("./testdata/server.pem", "./testdata/server.key")
	assert.NoError(t, err)

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caCertPool,
	}
	ts.StartTLS()
	defer ts.Close()

	res, err := client.Get(ts.URL)
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusOK)
}

func TestParseLabelRenamingAndAddingFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     string
		expected map[string]string
	}{
		{
			name:     "empty",
			args:     "",
			expected: map[string]string{},
		},
		{
			name: "single",
			args: "old=new",
			expected: map[string]string{
				"old": "new",
			},
		},
		{
			name: "multiple",
			args: "old=new,aged=pristine",
			expected: map[string]string{
				"old":  "new",
				"aged": "pristine",
			},
		},
		{
			name:     "empty value (ignored)",
			args:     "old",
			expected: map[string]string{},
		},
		{
			name: "one empty value",
			args: "old,aged=pristine",
			expected: map[string]string{
				"aged": "pristine",
			},
		},
		{
			name: "multiple equals signs",
			args: "old=aged=pristine",
			expected: map[string]string{
				"old": "aged=pristine",
			},
		},
	} {
		// Added and renaming have the same parsing logic for arguments.
		t.Run("added "+tc.name, func(t *testing.T) {
			result := getAddedFromArg(tc.args)
			assert.Equal(t, tc.expected, result)
		})

		t.Run("renamed "+tc.name, func(t *testing.T) {
			result := getRenamedFromArg(tc.args)
			assert.Equal(t, tc.expected, result)
		})
	}
}
