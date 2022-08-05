package grpcstats

import (
	"context"
	"fmt"

	"github.com/stripe/veneur/v14/scopedstatsd"
	"google.golang.org/grpc/stats"
)

var _ stats.Handler = &StatsHandler{}

type StatsHandler struct {
	IsClient bool
	Statsd   scopedstatsd.Client
}

func (handler *StatsHandler) TagRPC(
	ctx context.Context, info *stats.RPCTagInfo,
) context.Context {
	return ctx
}

func (handler *StatsHandler) HandleRPC(
	ctx context.Context, rpcStats stats.RPCStats,
) {
	// Do nothing.
}

func (handler *StatsHandler) TagConn(
	ctx context.Context, info *stats.ConnTagInfo,
) context.Context {
	return ctx
}

func (handler *StatsHandler) HandleConn(
	ctx context.Context, connStats stats.ConnStats,
) {
	switch connStats.(type) {
	case *stats.ConnBegin:
		handler.Statsd.Count("veneur_proxy.grpc.conn_begin", 1, []string{
			fmt.Sprintf("client:%t", handler.IsClient),
		}, 1.0)
	case *stats.ConnEnd:
		handler.Statsd.Count("veneur_proxy.grpc.conn_end", 1, []string{
			fmt.Sprintf("client:%t", handler.IsClient),
		}, 1.0)
	}
}
