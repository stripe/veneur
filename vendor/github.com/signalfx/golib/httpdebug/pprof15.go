// +build go1.5

package httpdebug

import (
	"net/http"
	"net/http/pprof"
)

func setupTrace(m *http.ServeMux) {
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
