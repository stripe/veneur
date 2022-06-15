package grpcstats_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stripe/veneur/v14/proxy/grpcstats"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"google.golang.org/grpc/stats"
)

func TestHandleConnBegin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	statsd := scopedstatsd.NewMockClient(ctrl)
	handler := grpcstats.StatsHandler{
		Statsd: statsd,
	}

	statsd.EXPECT().Count(
		gomock.Eq("veneur_proxy.grpc.conn_begin"),
		int64(1), []string{"client:false"}, 1.0)

	handler.HandleConn(context.Background(), &stats.ConnBegin{})
}

func TestHandleConnEnd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	statsd := scopedstatsd.NewMockClient(ctrl)
	handler := grpcstats.StatsHandler{
		Statsd: statsd,
	}

	statsd.EXPECT().Count(
		gomock.Eq("veneur_proxy.grpc.conn_end"),
		int64(1), []string{"client:false"}, 1.0)

	handler.HandleConn(context.Background(), &stats.ConnEnd{})
}
