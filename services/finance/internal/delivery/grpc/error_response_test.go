package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// Import proto package to register file descriptors.
	_ "github.com/mutugading/goapps-backend/gen/finance/v1"
)

func TestWrapErrorInResponse_FinanceMethods(t *testing.T) {
	err := status.Error(codes.Unauthenticated, "authentication required")

	methods := []string{
		"/finance.v1.UOMService/CreateUOM",
		"/finance.v1.UOMService/GetUOM",
		"/finance.v1.UOMService/UpdateUOM",
		"/finance.v1.UOMService/DeleteUOM",
		"/finance.v1.UOMService/ListUOMs",
		"/finance.v1.UOMService/ExportUOMs",
		"/finance.v1.UOMService/ImportUOMs",
		"/finance.v1.UOMService/DownloadTemplate",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			resp := wrapErrorInResponse(method, err)
			require.NotNil(t, resp, "wrapErrorInResponse returned nil for %s", method)

			// Verify the "base" field is set via protobuf reflection
			baseField := resp.ProtoReflect().Descriptor().Fields().ByName("base")
			require.NotNil(t, baseField, "response has no 'base' field for %s", method)

			baseMsg := resp.ProtoReflect().Get(baseField).Message()
			isSuccess := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("is_success")).Bool()
			statusCode := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("status_code")).String()
			message := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("message")).String()

			assert.False(t, isSuccess)
			assert.Equal(t, "401", statusCode)
			assert.Equal(t, "authentication required", message)
		})
	}
}

func TestWrapErrorInResponse_AllErrorCodes(t *testing.T) {
	tests := []struct {
		code       codes.Code
		msg        string
		httpStatus string
	}{
		{codes.Unauthenticated, "auth required", "401"},
		{codes.PermissionDenied, "access denied", "403"},
		{codes.NotFound, "not found", "404"},
		{codes.InvalidArgument, "bad request", "400"},
		{codes.AlreadyExists, "duplicate", "409"},
		{codes.Internal, "server error", "500"},
		{codes.ResourceExhausted, "rate limited", "429"},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			err := status.Error(tt.code, tt.msg)
			resp := wrapErrorInResponse("/finance.v1.UOMService/ListUOMs", err)
			require.NotNil(t, resp)

			baseField := resp.ProtoReflect().Descriptor().Fields().ByName("base")
			baseMsg := resp.ProtoReflect().Get(baseField).Message()
			statusCode := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("status_code")).String()
			message := baseMsg.Get(baseMsg.Descriptor().Fields().ByName("message")).String()

			assert.Equal(t, tt.httpStatus, statusCode)
			assert.Equal(t, tt.msg, message)
		})
	}
}

func TestWrapErrorInResponse_UnknownMethod(t *testing.T) {
	err := status.Error(codes.Unauthenticated, "auth required")
	resp := wrapErrorInResponse("/unknown.Service/Method", err)
	assert.Nil(t, resp, "should return nil for unknown methods")
}
