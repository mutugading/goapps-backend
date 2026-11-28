package grpc

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

// Auth context keys.
const (
	AuthUserIDKey      ContextKey = "auth_user_id"
	AuthUsernameKey    ContextKey = "auth_username"
	AuthRolesKey       ContextKey = "auth_roles"
	AuthPermissionsKey ContextKey = "auth_permissions"
)

// JWTClaims mirrors the IAM service JWT claims structure.
type JWTClaims struct {
	jwt.RegisteredClaims
	TokenType   string   `json:"token_type"`
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// TokenBlacklistChecker checks whether a token was revoked (IAM logout, shared Redis).
type TokenBlacklistChecker interface {
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
}

// PermissionsReader fetches a user's permissions from the IAM Redis cache.
type PermissionsReader interface {
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
}

func isPublicMethod(fullMethod string) bool {
	return strings.HasPrefix(fullMethod, "/grpc.health.v1.") ||
		strings.HasPrefix(fullMethod, "/grpc.reflection.")
}

// AuthInterceptor validates IAM-issued JWTs and attaches the caller identity
// with its payroll scopes (pyr_user_scope, D-06/D-11) to the context.
// Scopes are resolved per request so admin changes apply immediately.
// blacklist and permsReader are optional (nil = skipped), same as finance (D-73 (3)): a revoked JTI is
// rejected; permissions come from the IAM Redis cache and fall back to the JWT claims on a miss or error.
func AuthInterceptor(cfg *config.JWTConfig, resolver scope.Resolver, blacklist TokenBlacklistChecker, permsReader PermissionsReader) grpc.UnaryServerInterceptor {
	secret := []byte(cfg.AccessTokenSecret)

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		token, err := extractBearerToken(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization: %v", err)
		}

		claims, err := validateAccessToken(token, secret)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		if revoked(ctx, blacklist, claims.ID) {
			return nil, status.Error(codes.Unauthenticated, "token has been revoked")
		}
		perms := userPermissions(ctx, permsReader, claims)

		scopes, err := resolver.Resolve(ctx, claims.UserID)
		if err != nil {
			log.Error().Err(err).Str("userID", claims.UserID).Msg("failed to resolve payroll scope")
			return nil, status.Error(codes.Internal, "failed to resolve payroll scope")
		}

		ctx = context.WithValue(ctx, AuthUserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, AuthUsernameKey, claims.Username)
		ctx = context.WithValue(ctx, AuthRolesKey, claims.Roles)
		ctx = context.WithValue(ctx, AuthPermissionsKey, perms)
		ctx = scope.WithIdentity(ctx, scope.Identity{UserID: claims.UserID, Scopes: scopes})

		return handler(ctx, req)
	}
}

// revoked reports whether the token JTI is on the IAM blacklist. A Redis error fails open (short access TTL),
// same as finance.
func revoked(ctx context.Context, blacklist TokenBlacklistChecker, jti string) bool {
	if blacklist == nil || jti == "" {
		return false
	}
	hit, err := blacklist.IsBlacklisted(ctx, jti)
	if err != nil {
		log.Warn().Err(err).Msg("failed to check token blacklist")
	}
	return hit
}

// userPermissions returns the cached IAM permissions, or the JWT claims on a cache miss or error.
func userPermissions(ctx context.Context, reader PermissionsReader, claims *JWTClaims) []string {
	if reader == nil {
		return claims.Permissions
	}
	cached, err := reader.GetUserPermissions(ctx, claims.UserID)
	if err != nil {
		log.Warn().Err(err).Str("userID", claims.UserID).Msg("failed to fetch permissions from Redis, using JWT fallback")
		return claims.Permissions
	}
	if cached == nil {
		return claims.Permissions
	}
	return cached
}

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errors.New("no authorization header")
	}

	authHeader := values[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", errors.New("invalid authorization format")
	}

	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

func validateAccessToken(tokenString string, secret []byte) (*JWTClaims, error) {
	// IAM signs with HS256 only (iam/internal/infrastructure/jwt); exp is mandatory.
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(*jwt.Token) (any, error) {
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token expired")
		}
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	if claims.TokenType != "access" {
		return nil, errors.New("not an access token")
	}
	if claims.UserID == "" {
		return nil, errors.New("missing user_id")
	}
	return claims, nil
}

// GetUserIDFromCtx retrieves the user ID from context.
func GetUserIDFromCtx(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AuthUserIDKey).(string)
	return val, ok
}

// GetPermissionsFromCtx retrieves the permissions from context.
func GetPermissionsFromCtx(ctx context.Context) []string {
	perms, ok := ctx.Value(AuthPermissionsKey).([]string)
	if !ok {
		return nil
	}
	return perms
}

// HasPermission checks if the user has a specific permission.
func HasPermission(ctx context.Context, permission string) bool {
	return slices.Contains(GetPermissionsFromCtx(ctx), permission)
}

// IsSuperAdmin checks if the user has the SUPER_ADMIN role.
func IsSuperAdmin(ctx context.Context) bool {
	roles, ok := ctx.Value(AuthRolesKey).([]string)
	return ok && slices.Contains(roles, "SUPER_ADMIN")
}

// PermissionInterceptor enforces RBAC permission checks per RPC. Permission
// only gates the action; which rows are visible is decided by scope + RLS.
func PermissionInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isPublicMethod(info.FullMethod) || IsSuperAdmin(ctx) {
			return handler(ctx, req)
		}

		required, mapped := rpcPermissions[info.FullMethod]
		if !mapped || required == "" {
			// Fail closed: every business RPC must be mapped explicitly.
			log.Warn().Str("method", info.FullMethod).Msg("RPC without permission mapping denied")
			return nil, status.Error(codes.PermissionDenied, "permission denied: RPC not mapped")
		}

		if !HasPermission(ctx, required) {
			log.Warn().Str("method", info.FullMethod).Str("required", required).Msg("Permission denied")
			return nil, status.Errorf(codes.PermissionDenied, "permission denied: requires %s", required)
		}
		return handler(ctx, req)
	}
}

// rpcPermissions maps a fully-qualified gRPC method to the payroll.* permission
// code it requires (services/payroll/docs/permissions.md). Filled in alongside
// the business handlers; an unmapped RPC is denied (PermissionInterceptor).
var rpcPermissions = map[string]string{}
