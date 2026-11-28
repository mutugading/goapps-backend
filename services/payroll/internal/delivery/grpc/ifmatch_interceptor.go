package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// IfMatchInterceptor copies the HTTP If-Match header (forwarded by the gateway
// as metadata "if-match") into the request's `if_match` field when the body
// left it empty, so REST clients can use the standard header (D-16). It must
// run before the validation interceptor, which requires `if_match`.
func IfMatchInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		vals := md.Get("if-match")
		if !ok || len(vals) == 0 || vals[0] == "" {
			return handler(ctx, req)
		}
		if m, isMsg := req.(proto.Message); isMsg {
			rm := m.ProtoReflect()
			if f := rm.Descriptor().Fields().ByName("if_match"); f != nil && f.Kind() == protoreflect.StringKind && rm.Get(f).String() == "" {
				rm.Set(f, protoreflect.ValueOfString(vals[0]))
			}
		}
		return handler(ctx, req)
	}
}
