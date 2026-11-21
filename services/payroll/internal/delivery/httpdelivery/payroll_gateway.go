package httpdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
)

// NewGatewayMux builds the gRPC-Gateway handler that proxies PayrollService on
// grpcTarget. Header mapping (07 Konvensi umum, D-16):
//   - If-Match      -> gRPC metadata "if-match" (interceptor fills `if_match`)
//   - ETag          <- `data.etag` of the response
//   - HTTP status   <- `base.status_code` (the gRPC error interceptor folds
//     errors into a 200 response, so the status must be restored here)
func NewGatewayMux(ctx context.Context, grpcTarget string) (http.Handler, error) {
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithIncomingHeaderMatcher(incomingHeaderMatcher),
		runtime.WithForwardResponseOption(forwardResponse),
		runtime.WithErrorHandler(baseResponseErrorHandler),
	)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := payrollv1.RegisterPayrollServiceHandlerFromEndpoint(ctx, gwMux, grpcTarget, opts); err != nil {
		return nil, fmt.Errorf("failed to register Payroll gateway: %w", err)
	}
	return gwMux, nil
}

// incomingHeaderMatcher forwards If-Match (and Idempotency-Key, stored by
// T007e) besides the gateway defaults such as Authorization.
func incomingHeaderMatcher(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "if-match", "idempotency-key":
		return strings.ToLower(key), true
	}
	return runtime.DefaultHeaderMatcher(key)
}

// forwardResponse sets ETag from `data.etag` and restores the HTTP status of a
// failed response from `base.status_code`.
func forwardResponse(_ context.Context, w http.ResponseWriter, resp proto.Message) error {
	m := resp.ProtoReflect()
	if v := messageField(m, "data"); v != nil {
		if ef := v.Descriptor().Fields().ByName("etag"); ef != nil && ef.Kind() == protoreflect.StringKind {
			if etag := v.Get(ef).String(); etag != "" {
				w.Header().Set("ETag", strconv.Quote(strings.Trim(etag, `"`)))
			}
		}
	}
	if code := failedStatus(messageField(m, "base")); code != 0 {
		w.WriteHeader(code)
	}
	return nil
}

// messageField returns the singular message field name of m, or nil when absent/unset.
func messageField(m protoreflect.Message, name protoreflect.Name) protoreflect.Message {
	f := m.Descriptor().Fields().ByName(name)
	if f == nil || f.Kind() != protoreflect.MessageKind || f.IsList() || !m.Has(f) {
		return nil
	}
	return m.Get(f).Message()
}

// failedStatus returns the HTTP status of an unsuccessful BaseResponse, or 0.
func failedStatus(base protoreflect.Message) int {
	if base == nil {
		return 0
	}
	fields := base.Descriptor().Fields()
	okF, codeF := fields.ByName("is_success"), fields.ByName("status_code")
	if okF == nil || codeF == nil || base.Get(okF).Bool() {
		return 0
	}
	if code, err := strconv.Atoi(base.Get(codeF).String()); err == nil && code >= 400 && code < 600 {
		return code
	}
	return 0
}

// baseResponseErrorHandler writes gateway-level errors (bad JSON, unknown
// route, transport failures) in the standard BaseResponse envelope.
func baseResponseErrorHandler(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
	st := status.Convert(err)
	httpCode := runtime.HTTPStatusFromCode(st.Code())

	resp := map[string]any{
		"base": map[string]any{
			"is_success":        false,
			"status_code":       strconv.Itoa(httpCode),
			"message":           st.Message(),
			"validation_errors": []any{},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		log.Warn().Err(encErr).Msg("Failed to write error response")
	}
}
