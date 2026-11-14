package grpc

import (
	"context"

	"buf.build/go/protovalidate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// ValidationInterceptor enforces the buf.validate constraints declared in the
// proto files, so malformed requests never reach a handler (07: 400 validasi).
func ValidationInterceptor() (grpc.UnaryServerInterceptor, error) {
	v, err := protovalidate.New()
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if m, ok := req.(proto.Message); ok {
			if err := v.Validate(m); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}
		return handler(ctx, req)
	}, nil
}
