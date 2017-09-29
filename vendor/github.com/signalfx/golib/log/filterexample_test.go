package log

import (
	"expvar"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

func ExampleFilter() {
	console := NewLogfmtLogger(os.Stderr, Discard)
	filter := &RegexFilter{
		Log:             console,
		MissingValueKey: Msg,
		ErrCallback:     Panic,
	}
	logout := MultiFilter{
		Filters: []Filter{filter},
		PassTo:  console,
	}
	ticker := time.NewTicker(time.Millisecond * 500)
	finished := make(chan struct{})
	defer ticker.Stop()
	go func() {
		for {
			select {
			case t := <-ticker.C:
				logout.Log("attime", t, "id", 1, "Got a message!")
			case <-finished:
				return
			}
		}
	}()
	socket, err := net.Listen("tcp", "localhost:8182")
	IfErr(Panic, err)
	fmt.Fprintf(os.Stderr, "Listening on http://%s/debug/logs\n", socket.Addr().String())
	handler := &FilterChangeHandler{
		Filter: filter,
		Log:    console,
	}
	expvar.Publish("test", filter.Var())
	http.DefaultServeMux.Handle("/debug/logs", handler)
	IfErr(console, http.Serve(socket, nil))
}
