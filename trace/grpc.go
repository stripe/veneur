package trace

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type metadataReaderWriter struct {
	*metadata.MD
}

func (w metadataReaderWriter) Set(key, val string) {
	key = strings.ToLower(key)
	if strings.HasSuffix(key, "-bin") {
		val = string(base64.StdEncoding.EncodeToString([]byte(val)))
	}
	(*w.MD)[key] = append((*w.MD)[key], val)
}

func (w metadataReaderWriter) ForeachKey(handler func(key, val string) error) error {
	for k, vals := range *w.MD {
		for _, v := range vals {
			if err := handler(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func UnaryClientTracingInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		span, tctx := StartSpanFromContext(ctx, method)

		clientCtx := PersistSpanToMetadata(tctx, span, method)
		err := invoker(clientCtx, method, req, reply, cc, opts...)
		return err
	}
}

func UnaryServerTracingInterceptor(traceClient *Client) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		span, tctx := NewServerSpanFromContext(ctx, info.FullMethod)
		defer span.ClientFinish(traceClient)

		resp, err := handler(tctx, req)
		return resp, err
	}
}

// NewServerSpanFromContext tries to read tracing data from incoming contexts
// and then return a span and context for the caller to use.
func NewServerSpanFromContext(ctx context.Context, fullMethodName string) (*Span, context.Context) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.Pairs()
	}

	wireContext, _ := GlobalTracer.Extract(opentracing.TextMap, metadataReaderWriter{&md})
	// if err != nil && err != opentracing.ErrSpanContextNotFound {
	// 	log.Debug("Call with no context", "method", fullMethodName)
	// }
	span := GlobalTracer.StartSpan(fullMethodName, ext.RPCServerOption(wireContext)).(*Span)
	if span.Tags == nil {
		span.Tags = map[string]string{}
	}

	return span, span.Attach(ctx)
}

func PersistSpanToMetadata(ctx context.Context, span *Span, resource string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}
	mdrw := metadataReaderWriter{&md}
	mdrw.Set("traceid", strconv.FormatInt(span.TraceID, 10))
	mdrw.Set("spanid", strconv.FormatInt(span.SpanID, 10))
	mdrw.Set("parentid", strconv.FormatInt(span.ParentID, 10))

	ctxWithMetadata := metadata.NewOutgoingContext(ctx, md)
	return ctxWithMetadata
}
