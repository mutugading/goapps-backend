// Package password provides password hashing and validation utilities.
package password

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"

	"golang.org/x/crypto/argon2"
)

// Argon2id configuration parameters.
const (
	// Memory in KB (64 MB)
	memory = 64 * 1024
	// Number of iterations
	iterations = 3
	// Number of parallel threads
	parallelism = 2
	// Length of salt in bytes
	saltLength = 16
	// Length of hash output in bytes
	keyLength = 32
)

// Policy defines password strength requirements.
type Policy struct {
	MinLength        int
	RequireUppercase bool
	RequireLowercase bool
	RequireNumber    bool
	RequireSpecial   bool
}

// DefaultPolicy returns the default password policy.
func DefaultPolicy() Policy {
	return Policy{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   false,
	}
}

var (
	uppercaseRegex = regexp.MustCompile(`[A-Z]`)
	lowercaseRegex = regexp.MustCompile(`[a-z]`)
	numberRegex    = regexp.MustCompile(`[0-9]`)
	specialRegex   = regexp.MustCompile(`[!@#$%^&*(),.?":{}|<>]`)
)

// Validation errors.
var (
	ErrPasswordTooShort = errors.New("password is too short")
	ErrNoUppercase      = errors.New("password must contain at least one uppercase letter")
	ErrNoLowercase      = errors.New("password must contain at least one lowercase letter")
	ErrNoNumber         = errors.New("password must contain at least one number")
	ErrNoSpecial        = errors.New("password must contain at least one special character")
)

// Validate checks if a password meets the policy requirements.
func Validate(password string, policy Policy) error {
	if len(password) < policy.MinLength {
		return ErrPasswordTooShort
	}
	if policy.RequireUppercase && !uppercaseRegex.MatchString(password) {
		return ErrNoUppercase
	}
	if policy.RequireLowercase && !lowercaseRegex.MatchString(password) {
		return ErrNoLowercase
	}
	if policy.RequireNumber && !numberRegex.MatchString(password) {
		return ErrNoNumber
	}
	if policy.RequireSpecial && !specialRegex.MatchString(password) {
		return ErrNoSpecial
	}
	return nil
}

// Hash creates an Argon2id hash of the password.
func Hash(password string) (string, error) {
	// Generate random salt
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// Generate hash
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	// Encode as: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return "$argon2id$v=19$m=65536,t=3,p=2$" + b64Salt + "$" + b64Hash, nil
}

// Verify checks if a password matches the hash.
func Verify(password, encodedHash string) (bool, error) {
	// Parse the encoded hash
	salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	// Compute hash of provided password
	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	// Constant-time comparison
	if len(hash) != len(computedHash) {
		return false, nil
	}

	var result byte
	for i := 0; i < len(hash); i++ {
		result |= hash[i] ^ computedHash[i]
	}

	return result == 0, nil
}

func decodeHash(encodedHash string) ([]byte, []byte, error) {
	// Expected format: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
	var salt, hash []byte

	// Simple parsing - split by $
	parts := splitHash(encodedHash)
	if len(parts) != 6 {
		return nil, nil, errors.New("invalid hash format")
	}

	// Decode salt and hash
	var err error
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, err
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, err
	}

	return salt, hash, nil
}

func splitHash(s string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '$' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return parts
}

// GenerateTemporaryPassword generates a random temporary password.
func GenerateTemporaryPassword(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// GenerateOTP generates a random numeric OTP code.
func GenerateOTP(length int) (string, error) {
	const digits = "0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b), nil
}
