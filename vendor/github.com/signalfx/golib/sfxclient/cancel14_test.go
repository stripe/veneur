// +build !go1.5

package sfxclient

import "net/http"
import "golang.org/x/net/context"
import "testing"

func cancelChanFromReq(req *http.Request) <-chan struct{} {
	return make(chan struct{})
}

func timeoutString() string {
	return "use of closed network connection"
}

func TestCoverageForNoForwardTransport(t *testing.T) {
	req, _ := http.NewRequest("", "", nil)
	ctx := context.Background()
	h := HTTPSink{}
	h.Client.Transport = nil
	if h.withCancel(ctx, req) == nil {
		t.Error("Expected an error")
	}
}
