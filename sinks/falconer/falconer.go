package falconer

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/sinks/grpsink"
	"google.golang.org/grpc"
)

func NewSpanSink(ctx context.Context, target string, log *logrus.Logger, opts ...grpc.DialOption) (*grpsink.GRPCSpanSink, error) {
	return grpsink.NewGRPCSpanSink(ctx, target, "falconer", log, opts...)
}
