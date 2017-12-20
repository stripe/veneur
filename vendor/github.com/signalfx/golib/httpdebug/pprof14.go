// +build !go1.5

package httpdebug

import (
	"net/http"
)

func setupTrace(m *http.ServeMux) {
	// Ignored.  Trace not supported in < 1.5
}
