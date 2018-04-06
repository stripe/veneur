// +build go1.9

package grpsink

import (
	"context"

	ocontext "golang.org/x/net/context"
)

func convertContext(ctx context.Context) ocontext.Context {
	return ctx
}
