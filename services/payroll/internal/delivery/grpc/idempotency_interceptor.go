package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/idempotency"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

const (
	maxIdempotencyKeyLen = 128
	completeAttempts     = 3
)

// IdempotencyInterceptor stores the successful result of a mutating RPC under
// the Idempotency-Key header (forwarded as metadata, T007b) and replays it when
// the same user repeats the key within 24 hours (D-19). Same key with another
// body or another scope is IDEMPOTENCY_KEY_CONFLICT; a still-running first
// request is IDEMPOTENCY_IN_PROGRESS. Requests without the header, and reads
// (Get*/List*), pass through untouched. It runs after validation.
func IdempotencyInterceptor(store idempotency.Store) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		key := idempotencyHeader(ctx)
		msg, isMsg := req.(proto.Message)
		if key == "" || !isMsg || isReadMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		userID, ok := GetUserIDFromCtx(ctx)
		if !ok || userID == "" {
			return handler(ctx, req)
		}
		if len(key) > maxIdempotencyKeyLen {
			return nil, ToStatusError(apperr.New(apperr.CodeValidation, "Idempotency-Key terlalu panjang (maks 128)", nil))
		}

		reqBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to hash request")
		}
		k := idempotency.Key{UserID: userID, Method: info.FullMethod, Key: key}

		state, claim, stored, err := store.Begin(ctx, k, sha256Hex(reqBytes), callerRightsHash(ctx))
		if err != nil {
			log.Error().Err(err).Str("method", info.FullMethod).Msg("idempotency begin failed")
			return nil, status.Error(codes.Internal, "idempotency store unavailable")
		}
		switch state {
		case idempotency.Replay:
			return unmarshalStored(info.FullMethod, stored)
		case idempotency.Conflict:
			return nil, ToStatusError(apperr.New(apperr.CodeIdemConflict,
				"Idempotency-Key sudah dipakai untuk request atau scope yang berbeda", nil))
		case idempotency.InProgress:
			return nil, ToStatusError(apperr.New(apperr.CodeIdemInProgress,
				"request dengan Idempotency-Key ini masih diproses", nil))
		case idempotency.Started:
		}

		return runIdempotent(ctx, store, k, claim, info.FullMethod, req, handler)
	}
}

// runIdempotent runs the handler of a claimed key and settles the key: a
// successful response is stored, anything else (error, failed base, panic)
// releases the key for a retry.
func runIdempotent(ctx context.Context, store idempotency.Store, k idempotency.Key, claim, method string, req any, handler grpc.UnaryHandler) (resp any, err error) {
	// The request context may be cancelled by the time we settle the key.
	settle := context.WithoutCancel(ctx)
	abort := func() {
		if aerr := store.Abort(settle, k, claim); aerr != nil {
			log.Error().Err(aerr).Str("method", method).Msg("idempotency abort failed")
		}
	}
	settled := false
	defer func() {
		if !settled {
			abort() // handler panicked; RecoveryInterceptor turns it into an error
		}
	}()

	resp, err = handler(ctx, req)
	settled = true
	out, isOut := resp.(proto.Message)
	if err != nil || !isOut || !responseSucceeded(out) {
		abort()
		return resp, err
	}
	b, merr := proto.Marshal(out)
	for attempt := 0; merr == nil && attempt < completeAttempts; attempt++ {
		if merr = store.Complete(settle, k, claim, b); merr == nil {
			return resp, nil
		}
	}
	// The write happened; leave the key IN_PROGRESS (stale after 2 minutes) rather than replay nothing.
	log.Error().Err(merr).Str("method", method).Msg("idempotency complete failed")
	return resp, nil
}

// callerRightsHash binds a stored result to what the caller may see: scopes and
// permissions, order-independent. A replay after either changed is a conflict,
// so a response built with broader rights (e.g. decrypted PII, D-17) is never
// replayed to a user who lost them.
func callerRightsHash(ctx context.Context) string {
	var scopes []string
	if id, ok := scope.FromContext(ctx); ok {
		for _, s := range id.Scopes {
			scopes = append(scopes, s.String())
		}
	}
	perms := slices.Clone(GetPermissionsFromCtx(ctx))
	slices.Sort(scopes)
	slices.Sort(perms)
	super := "0"
	if IsSuperAdmin(ctx) {
		super = "1"
	}
	return sha256Hex([]byte(strings.Join(scopes, ",") + "|" + strings.Join(perms, ",") + "|" + super))
}

func idempotencyHeader(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get("idempotency-key"); len(v) > 0 {
		return strings.TrimSpace(v[0])
	}
	return ""
}

// isReadMethod: reads never need a stored result.
func isReadMethod(fullMethod string) bool {
	name := fullMethod[strings.LastIndex(fullMethod, "/")+1:]
	return strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List")
}

// responseSucceeded: handlers may fold a failure into `base` and return nil error.
func responseSucceeded(m proto.Message) bool {
	rm := m.ProtoReflect()
	f := rm.Descriptor().Fields().ByName("base")
	if f == nil || f.Kind() != protoreflect.MessageKind || !rm.Has(f) {
		return true
	}
	base := rm.Get(f).Message()
	ok := base.Descriptor().Fields().ByName("is_success")
	return ok == nil || base.Get(ok).Bool()
}

func unmarshalStored(fullMethod string, b []byte) (any, error) {
	out := newResponse(fullMethod)
	if out == nil {
		return nil, status.Error(codes.Internal, "cannot replay idempotent response")
	}
	if err := proto.Unmarshal(b, out); err != nil {
		return nil, status.Error(codes.Internal, "cannot decode stored idempotent response")
	}
	return out, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
