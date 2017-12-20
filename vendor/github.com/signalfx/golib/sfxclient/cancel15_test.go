// +build go1.5

package sfxclient

import "net/http"

func cancelChanFromReq(req *http.Request) <-chan struct{} {
	return req.Cancel
}

func timeoutString() string {
	return "Timeout exceeded"
}
