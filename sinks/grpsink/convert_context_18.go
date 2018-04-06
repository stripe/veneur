// +build !go1.9

package grpsink

import (
	"context"

	ocontext "golang.org/x/net/context"
)

func convertContext(ctx context.Context) ocontext.Context {
	octx, cf := ocontext.WithCancel(ocontext.Background())

	// This is ok to do because we only use it on sink startup, not on a hot
	// path. Otherwise it'd leak goroutines like crazy.
	go func() {
		<-ctx.Done()
		cf()
	}()
	return octx
}
