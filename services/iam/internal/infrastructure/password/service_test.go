package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHash_And_Verify(t *testing.T) {
	pw := "SecurePassword123!"

	hash, err := Hash(pw)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$"))

	// Correct password should verify.
	match, err := Verify(pw, hash)
	require.NoError(t, err)
	assert.True(t, match)

	// Wrong password should not verify.
	match, err = Verify("WrongPassword", hash)
	require.NoError(t, err)
	assert.False(t, match)
}

func TestHash_UniquePerCall(t *testing.T) {
	pw := "SamePassword123"

	hash1, err := Hash(pw)
	require.NoError(t, err)

	hash2, err := Hash(pw)
	require.NoError(t, err)

	// Different salt should produce different hashes.
	assert.NotEqual(t, hash1, hash2)

	// Both should verify against the same password.
	m1, _ := Verify(pw, hash1)
	m2, _ := Verify(pw, hash2)
	assert.True(t, m1)
	assert.True(t, m2)
}

func TestVerify_InvalidFormat(t *testing.T) {
	_, err := Verify("password", "not-a-valid-hash")
	assert.Error(t, err)
}

func TestVerifyBcryptLegacy(t *testing.T) {
	pw := "LegacyPassword123"

	// Create a bcrypt hash.
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	require.NoError(t, err)

	// Correct password should verify.
	match, err := VerifyBcryptLegacy(pw, string(bcryptHash))
	require.NoError(t, err)
	assert.True(t, match)

	// Wrong password should not verify.
	match, err = VerifyBcryptLegacy("WrongPassword", string(bcryptHash))
	require.NoError(t, err)
	assert.False(t, match)
}

func TestVerifyBcryptLegacy_InvalidHash(t *testing.T) {
	_, err := VerifyBcryptLegacy("password", "not-a-bcrypt-hash")
	assert.Error(t, err)
}

func TestValidate(t *testing.T) {
	policy := DefaultPolicy()

	tests := []struct {
		name    string
		pw      string
		wantErr error
	}{
		{"valid", "SecurePass1", nil},
		{"too short", "Short1", ErrPasswordTooShort},
		{"no uppercase", "nouppercase1", ErrNoUppercase},
		{"no lowercase", "NOLOWERCASE1", ErrNoLowercase},
		{"no number", "NoNumberHere", ErrNoNumber},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.pw, policy)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_SpecialCharRequired(t *testing.T) {
	policy := Policy{
		MinLength:      8,
		RequireSpecial: true,
	}

	err := Validate("NoSpecial1A", policy)
	assert.ErrorIs(t, err, ErrNoSpecial)

	err = Validate("Special1!A", policy)
	assert.NoError(t, err)
}

func TestGenerateOTP(t *testing.T) {
	otp, err := GenerateOTP(6)
	require.NoError(t, err)
	assert.Len(t, otp, 6)

	// All characters should be digits.
	for _, c := range otp {
		assert.True(t, c >= '0' && c <= '9', "expected digit, got %c", c)
	}
}

func TestGenerateTemporaryPassword(t *testing.T) {
	pw, err := GenerateTemporaryPassword(16)
	require.NoError(t, err)
	assert.Len(t, pw, 16)
}
