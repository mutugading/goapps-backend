// Package grpc provides gRPC server implementation for the Payroll service.
package grpc

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
)

// StructuredErrorInterceptor wraps a real gRPC error (returned as `err != nil`)
// into a typed response with BaseResponse, so gRPC and HTTP (gateway) clients
// get a consistent envelope. It only fires for RPCs that actually return
// `err != nil` -- handlers that fold an error into a response's `base` field and
// return `(resp, nil)` bypass this interceptor entirely (see backend CLAUDE.md §5).
func StructuredErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		if structured := wrapErrorInResponse(info.FullMethod, err); structured != nil {
			return structured, nil
		}

		return nil, err
	}
}

// wrapErrorInResponse dynamically creates the correct response message for the
// given gRPC method and sets its "base" field from the error.
func wrapErrorInResponse(fullMethod string, grpcErr error) proto.Message {
	resp := newResponse(fullMethod)
	if resp == nil {
		return nil
	}

	baseFieldDesc := resp.ProtoReflect().Descriptor().Fields().ByName("base")
	if baseFieldDesc == nil {
		return nil
	}

	st := status.Convert(grpcErr)
	httpStatus := grpcCodeToHTTPStatus(st.Code())
	if code, ok := CodeFromError(grpcErr); ok {
		// Exact 07 status (e.g. 422 business rule) instead of the coarse gRPC mapping.
		httpStatus = code.HTTPStatus()
	}
	base := &commonv1.BaseResponse{
		IsSuccess:  false,
		StatusCode: fmt.Sprintf("%d", httpStatus),
		Message:    st.Message(),
	}

	resp.ProtoReflect().Set(baseFieldDesc, protoreflect.ValueOfMessage(base.ProtoReflect()))

	return resp
}

// grpcCodeToHTTPStatus maps gRPC status codes to HTTP status codes.
func grpcCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return 200
	case codes.InvalidArgument:
		return 400
	case codes.Unauthenticated:
		return 401
	case codes.PermissionDenied:
		return 403
	case codes.NotFound:
		return 404
	case codes.AlreadyExists:
		return 409
	case codes.ResourceExhausted:
		return 429
	case codes.FailedPrecondition:
		return 412
	case codes.Unimplemented:
		return 501
	case codes.Unavailable:
		return 503
	case codes.DeadlineExceeded:
		return 504
	default:
		return 500
	}
}

// newResponse returns an empty response message of the given gRPC method, or nil.
func newResponse(fullMethod string) proto.Message {
	// Parse "/package.Service/Method" -> ["package.Service", "Method"].
	parts := strings.Split(strings.TrimPrefix(fullMethod, "/"), "/")
	if len(parts) != 2 {
		return nil
	}

	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(parts[0]))
	if err != nil {
		return nil
	}

	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil
	}

	methodDesc := svc.Methods().ByName(protoreflect.Name(parts[1]))
	if methodDesc == nil {
		return nil
	}

	outputType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Output().FullName())
	if err != nil {
		return nil
	}
	return outputType.New().Interface()
}
