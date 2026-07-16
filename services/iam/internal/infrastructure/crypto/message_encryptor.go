// Package crypto provides AES-256-GCM encryption for chat conversation keys
// and message bodies, wrapping per-conversation keys with a master key.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Encryptor handles AES-256-GCM encryption for chat messages.
type Encryptor struct {
	masterGCM cipher.AEAD
}

// NewEncryptor creates an Encryptor from a 32-byte master key.
func NewEncryptor(masterKey []byte) (*Encryptor, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("crypto: master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: create master cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create master GCM: %w", err)
	}
	return &Encryptor{masterGCM: gcm}, nil
}

// GenerateConversationKey generates a random 32-byte AES key for a new conversation.
func (e *Encryptor) GenerateConversationKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: generate conv key: %w", err)
	}
	return key, nil
}

// EncryptConversationKey encrypts a conversation key with the master key.
func (e *Encryptor) EncryptConversationKey(convKey []byte) ([]byte, error) {
	return e.seal(e.masterGCM, convKey)
}

// DecryptConversationKey decrypts a stored conversation key using the master key.
func (e *Encryptor) DecryptConversationKey(encConvKey []byte) ([]byte, error) {
	return e.open(e.masterGCM, encConvKey)
}

// EncryptMessage encrypts a plaintext message body with the given conversation key.
func (e *Encryptor) EncryptMessage(convKey []byte, plaintext string) ([]byte, error) {
	gcm, err := e.convGCM(convKey)
	if err != nil {
		return nil, err
	}
	return e.seal(gcm, []byte(plaintext))
}

// DecryptMessage decrypts a stored message ciphertext using the conversation key.
func (e *Encryptor) DecryptMessage(convKey, ciphertext []byte) (string, error) {
	gcm, err := e.convGCM(convKey)
	if err != nil {
		return "", err
	}
	plain, err := e.open(gcm, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (e *Encryptor) convGCM(convKey []byte) (cipher.AEAD, error) {
	if len(convKey) != 32 {
		return nil, fmt.Errorf("crypto: conv key must be 32 bytes, got %d", len(convKey))
	}
	block, err := aes.NewCipher(convKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: create conv cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create conv GCM: %w", err)
	}
	return gcm, nil
}

func (e *Encryptor) seal(gcm cipher.AEAD, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (e *Encryptor) open(gcm cipher.AEAD, data []byte) ([]byte, error) {
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ct := data[:ns], data[ns:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt failed (tampered?): %w", err)
	}
	return plain, nil
}
