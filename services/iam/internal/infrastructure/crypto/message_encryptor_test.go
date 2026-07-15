package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

func TestEncryptor_RoundTrip(t *testing.T) {
	masterKey := make([]byte, 32)
	enc, err := crypto.NewEncryptor(masterKey)
	require.NoError(t, err)

	convKey, err := enc.GenerateConversationKey()
	require.NoError(t, err)
	assert.Len(t, convKey, 32)

	encConvKey, err := enc.EncryptConversationKey(convKey)
	require.NoError(t, err)

	decConvKey, err := enc.DecryptConversationKey(encConvKey)
	require.NoError(t, err)
	assert.Equal(t, convKey, decConvKey)

	ciphertext, err := enc.EncryptMessage(convKey, "hello world")
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)
	assert.NotContains(t, string(ciphertext), "hello world")

	plaintext, err := enc.DecryptMessage(convKey, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "hello world", plaintext)
}

func TestEncryptor_TamperDetection(t *testing.T) {
	masterKey := make([]byte, 32)
	enc, err := crypto.NewEncryptor(masterKey)
	require.NoError(t, err)

	convKey, err := enc.GenerateConversationKey()
	require.NoError(t, err)

	ct, err := enc.EncryptMessage(convKey, "secret")
	require.NoError(t, err)

	ct[len(ct)-1] ^= 0xFF

	_, err = enc.DecryptMessage(convKey, ct)
	assert.Error(t, err, "tampered ciphertext must fail")
}

func TestEncryptor_InvalidMasterKey(t *testing.T) {
	_, err := crypto.NewEncryptor([]byte("too-short"))
	assert.Error(t, err)
}
