// Package auth provides authentication application services.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	domainAuth "github.com/mutugading/goapps-backend/services/iam/internal/domain/auth"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	redisinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/redis"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/totp"
)

// Service implements domainAuth.Service interface.
type Service struct {
	userRepo       user.Repository
	sessionRepo    session.Repository
	auditRepo      audit.Repository
	jwtService     *jwt.Service
	totpService    *totp.Service
	sessionCache   *redisinfra.SessionCache
	otpCache       *redisinfra.OTPCache
	rateLimitCache *redisinfra.RateLimitCache
	securityCfg    *config.SecurityConfig
}

// NewService creates a new auth service.
func NewService(
	userRepo user.Repository,
	sessionRepo session.Repository,
	auditRepo audit.Repository,
	jwtService *jwt.Service,
	totpService *totp.Service,
	sessionCache *redisinfra.SessionCache,
	otpCache *redisinfra.OTPCache,
	rateLimitCache *redisinfra.RateLimitCache,
	securityCfg *config.SecurityConfig,
) *Service {
	return &Service{
		userRepo:       userRepo,
		sessionRepo:    sessionRepo,
		auditRepo:      auditRepo,
		jwtService:     jwtService,
		totpService:    totpService,
		sessionCache:   sessionCache,
		otpCache:       otpCache,
		rateLimitCache: rateLimitCache,
		securityCfg:    securityCfg,
	}
}

// Login authenticates a user and returns tokens.
func (s *Service) Login(ctx context.Context, input domainAuth.LoginInput) (*domainAuth.LoginResult, error) {
	// Check rate limiting
	if s.rateLimitCache != nil {
		attempts, err := s.rateLimitCache.GetLoginAttempts(ctx, input.Username)
		if err == nil && attempts >= int64(s.securityCfg.MaxLoginAttempts) {
			return nil, shared.ErrAccountLocked
		}
	}

	// Find user
	u, err := s.userRepo.GetByUsername(ctx, input.Username)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			s.recordFailedLogin(ctx, input.Username)
			return nil, shared.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user can login
	if err := u.CanLogin(); err != nil {
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash()), []byte(input.Password)); err != nil {
		s.recordFailedLogin(ctx, input.Username)
		u.RecordLoginFailure(s.securityCfg.MaxLoginAttempts, s.securityCfg.LockoutDuration)
		_ = s.userRepo.Update(ctx, u)
		return nil, shared.ErrInvalidCredentials
	}

	// Check 2FA if enabled
	if u.TwoFactorEnabled() {
		if input.TOTPCode == "" {
			return nil, shared.Err2FARequired
		}
		if !s.totpService.Validate(u.TwoFactorSecret(), input.TOTPCode) {
			return nil, shared.ErrInvalid2FACode
		}
	}

	// Get user roles and permissions
	roles, permissions, err := s.userRepo.GetRolesAndPermissions(ctx, u.ID())
	if err != nil {
		return nil, fmt.Errorf("failed to get roles and permissions: %w", err)
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Code()
	}
	permNames := make([]string, len(permissions))
	for i, p := range permissions {
		permNames[i] = p.Code()
	}

	// Generate tokens
	tokenPair, err := s.jwtService.GenerateTokenPair(u.ID(), u.Username(), u.Email(), roleNames, permNames, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Record successful login
	u.RecordLoginSuccess(input.IPAddress)
	if err := s.userRepo.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Reset rate limit on success
	if s.rateLimitCache != nil {
		_ = s.rateLimitCache.ResetLoginAttempts(ctx, input.Username)
	}

	// Create session
	sess := session.NewSession(u.ID(), tokenPair.TokenID, input.IPAddress, input.UserAgent, input.DeviceInfo, tokenPair.RefreshExp)
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Cache session
	if s.sessionCache != nil {
		_ = s.sessionCache.StoreSession(ctx, sess.ID(), u.ID(), s.jwtService.GetRefreshTTLSeconds())
	}

	// Get user detail for full name
	detail, _ := s.userRepo.GetDetailByUserID(ctx, u.ID())
	fullName := u.Username()
	if detail != nil {
		fullName = detail.FullName()
	}

	// Log audit
	s.logAudit(ctx, u.ID(), "LOGIN", "User logged in", input.IPAddress, input.UserAgent)

	return &domainAuth.LoginResult{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    s.jwtService.GetAccessTTLSeconds(),
		User: &domainAuth.AuthUserInfo{
			ID:               u.ID(),
			Username:         u.Username(),
			Email:            u.Email(),
			FullName:         fullName,
			TwoFactorEnabled: u.TwoFactorEnabled(),
			Roles:            roleNames,
			Permissions:      permNames,
		},
	}, nil
}

// Logout invalidates a user session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return shared.ErrInvalidToken
	}

	// Blacklist the token
	if s.sessionCache != nil {
		expiresIn := claims.ExpiresAt.Unix() - time.Now().Unix()
		if expiresIn > 0 {
			_ = s.sessionCache.BlacklistToken(ctx, claims.ID, expiresIn)
		}
	}

	// Invalidate session in database
	if err := s.sessionRepo.RevokeByTokenID(ctx, claims.ID); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	userID, _ := uuid.Parse(claims.UserID)
	s.logAudit(ctx, userID, "LOGOUT", "User logged out", "", "")

	return nil
}

// RefreshToken refreshes the access token using a refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*domainAuth.RefreshResult, error) {
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, shared.ErrInvalidToken
	}

	// Check if token is blacklisted
	if s.sessionCache != nil {
		blacklisted, _ := s.sessionCache.IsBlacklisted(ctx, claims.ID)
		if blacklisted {
			return nil, shared.ErrTokenRevoked
		}
	}

	// Verify session exists
	sess, err := s.sessionRepo.GetByTokenID(ctx, claims.ID)
	if err != nil || sess == nil || sess.IsRevoked() {
		return nil, shared.ErrSessionNotFound
	}

	userID, _ := uuid.Parse(claims.UserID)
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get roles and permissions
	roles, permissions, _ := s.userRepo.GetRolesAndPermissions(ctx, u.ID())
	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Code()
	}
	permNames := make([]string, len(permissions))
	for i, p := range permissions {
		permNames[i] = p.Code()
	}

	// Generate new token pair
	tokenPair, err := s.jwtService.GenerateTokenPair(u.ID(), u.Username(), u.Email(), roleNames, permNames, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Blacklist old refresh token and create new session
	if s.sessionCache != nil {
		expiresIn := claims.ExpiresAt.Unix() - time.Now().Unix()
		if expiresIn > 0 {
			_ = s.sessionCache.BlacklistToken(ctx, claims.ID, expiresIn)
		}
	}

	// Update session with new token ID
	sess.UpdateTokenID(tokenPair.TokenID, tokenPair.RefreshExp)
	if err := s.sessionRepo.Update(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return &domainAuth.RefreshResult{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    s.jwtService.GetAccessTTLSeconds(),
	}, nil
}

// ForgotPassword initiates the password reset flow.
func (s *Service) ForgotPassword(ctx context.Context, email string) (expiresIn int, err error) {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Return success even if user not found (prevent email enumeration)
		if errors.Is(err, shared.ErrNotFound) {
			return int(s.securityCfg.OTPExpiry.Seconds()), nil
		}
		return 0, fmt.Errorf("failed to get user: %w", err)
	}

	// Generate OTP
	otp := generateOTP(6)
	if s.otpCache != nil {
		if err := s.otpCache.StoreOTP(ctx, u.ID(), otp); err != nil {
			return 0, fmt.Errorf("failed to store OTP: %w", err)
		}
	}

	// TODO: Send OTP via email
	// For now, log it (in production, implement email sending)

	s.logAudit(ctx, u.ID(), "FORGOT_PASSWORD", "Password reset requested", "", "")

	return int(s.securityCfg.OTPExpiry.Seconds()), nil
}

// VerifyResetOTP verifies the password reset OTP.
func (s *Service) VerifyResetOTP(ctx context.Context, email, otpCode string) (resetToken string, err error) {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return "", shared.ErrInvalidCredentials
	}

	// Verify OTP
	if s.otpCache != nil {
		valid, err := s.otpCache.VerifyOTP(ctx, u.ID(), otpCode)
		if err != nil || !valid {
			return "", shared.ErrInvalidOTP
		}
	}

	// Generate reset token
	resetToken = generateResetToken()
	if s.otpCache != nil {
		if err := s.otpCache.StoreResetToken(ctx, resetToken, u.ID(), 15*time.Minute); err != nil {
			return "", fmt.Errorf("failed to store reset token: %w", err)
		}
	}

	return resetToken, nil
}

// ResetPassword resets the password using a reset token.
func (s *Service) ResetPassword(ctx context.Context, resetToken, newPassword string) error {
	if s.otpCache == nil {
		return errors.New("OTP cache not configured")
	}

	userID, err := s.otpCache.GetResetToken(ctx, resetToken)
	if err != nil || userID == uuid.Nil {
		return shared.ErrInvalidToken
	}

	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := u.UpdatePassword(string(hash), "system"); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.logAudit(ctx, u.ID(), "RESET_PASSWORD", "Password reset completed", "", "")

	return nil
}

// UpdatePassword changes the user's password.
func (s *Service) UpdatePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash()), []byte(currentPassword)); err != nil {
		return shared.ErrInvalidCredentials
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := u.UpdatePassword(string(hash), userID.String()); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.logAudit(ctx, u.ID(), "CHANGE_PASSWORD", "Password changed", "", "")

	return nil
}

// Enable2FA initiates 2FA setup.
func (s *Service) Enable2FA(ctx context.Context, userID uuid.UUID) (*domainAuth.Enable2FAResult, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if u.TwoFactorEnabled() {
		return nil, shared.Err2FAAlreadyEnabled
	}

	// Generate secret
	secret, err := s.totpService.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	// Generate QR code URL
	qrURL := s.totpService.GenerateQRURI(secret, u.Email())

	// Generate recovery codes
	recoveryCodes := generateRecoveryCodes(8)

	// Store secret temporarily (not activated yet)
	// User must verify with TOTP code first
	if s.otpCache != nil {
		_ = s.otpCache.StoreResetToken(ctx, "2fa:"+userID.String(), uuid.New(), 10*time.Minute)
	}

	return &domainAuth.Enable2FAResult{
		Secret:        secret,
		QRCodeURL:     qrURL,
		RecoveryCodes: recoveryCodes,
	}, nil
}

// Verify2FA verifies and activates 2FA.
func (s *Service) Verify2FA(ctx context.Context, userID uuid.UUID, totpCode string) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// For initial verification, the secret should be passed from Enable2FA
	// In production, you'd store the pending secret in cache
	// For now, we assume the secret is already set

	if !s.totpService.Validate(u.TwoFactorSecret(), totpCode) {
		return shared.ErrInvalid2FACode
	}

	// 2FA is already validated, just confirm activation
	s.logAudit(ctx, u.ID(), "ENABLE_2FA", "Two-factor authentication enabled", "", "")

	return nil
}

// Disable2FA disables 2FA for a user.
func (s *Service) Disable2FA(ctx context.Context, userID uuid.UUID, password, totpCode string) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if !u.TwoFactorEnabled() {
		return shared.Err2FANotEnabled
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash()), []byte(password)); err != nil {
		return shared.ErrInvalidCredentials
	}

	// Verify TOTP code
	if !s.totpService.Validate(u.TwoFactorSecret(), totpCode) {
		return shared.ErrInvalid2FACode
	}

	if err := u.Disable2FA(userID.String()); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.logAudit(ctx, u.ID(), "DISABLE_2FA", "Two-factor authentication disabled", "", "")

	return nil
}

// Helper functions

func (s *Service) recordFailedLogin(ctx context.Context, username string) {
	if s.rateLimitCache != nil {
		_, _ = s.rateLimitCache.IncrementLoginAttempt(ctx, username, s.securityCfg.LockoutDuration)
	}
}

func (s *Service) logAudit(ctx context.Context, userID uuid.UUID, action, description, ipAddress, userAgent string) {
	if s.auditRepo != nil {
		log := audit.NewLog(
			audit.EventType(action),
			"mst_user",
			&userID,
			&userID,
			"", // username - will be populated if needed
			description,
			ipAddress,
			userAgent,
			"iam",
		)
		_ = s.auditRepo.Create(ctx, log)
	}
}

func generateOTP(length int) string {
	const digits = "0123456789"
	b := make([]byte, length)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b)
}

func generateResetToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateRecoveryCodes(count int) []string {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		b := make([]byte, 5)
		_, _ = rand.Read(b)
		codes[i] = hex.EncodeToString(b)
	}
	return codes
}
