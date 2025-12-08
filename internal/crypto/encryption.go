package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

const (
	KeySize   = 32 // AES-256 requires 32 bytes
	NonceSize = 12 // GCM standard nonce size
	SaltSize  = 16 // Salt for key derivation
)

// Encrypt encrypts plaintext using AES-256-GCM
// Returns base64-encoded: nonce + ciphertext
func Encrypt(plaintext string, key []byte) (string, error) {
	// 1. Validate key size (must be 32 bytes)
	if len(key) != KeySize {
		return "", errors.New("encryption key must be 32 bytes long")
	}

	// 2. Create AES cipher
	aesCipher, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// 3. Create GCM mode
	aesGcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return "", err
	}

	// 4. Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// 5. Encrypt data
	ciphertext := aesGcm.Seal(nil, nonce, []byte(plaintext), nil)

	// 6. Prepend nonce to ciphertext
	result := append(nonce, ciphertext...)

	// 7. Encode as base64
	return base64.StdEncoding.EncodeToString(result), nil
}

// Decrypt decrypts AES-256-GCM encrypted data
func Decrypt(ciphertext string, key []byte) (string, error) {
	// 1. Validate key size
	if len(key) != KeySize {
		return "", errors.New("encryption key must be 32 bytes long")
	}

	// 2. Decode from base64
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	// 3. Check minimum length (nonce + at least some data)
	if len(decoded) < NonceSize {
		return "", errors.New("ciphertext too short")
	}

	// 4. Extract nonce (first 12 bytes)
	nonce := decoded[:NonceSize]

	// 5. Extract ciphertext (remaining bytes)
	cipherData := decoded[NonceSize:]

	// 6. Create AES cipher
	aesCipher, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// 7. Create GCM mode
	aesGcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return "", err
	}

	// 8. Decrypt data
	plaintext, err := aesGcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", errors.New("decryption failed: invalid key or corrupted data")
	}

	return string(plaintext), nil
}

// GenerateKey generates a random 32-byte encryption key
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	_, err := io.ReadFull(rand.Reader, key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// DeriveKey derives an encryption key from a password using Argon2
func DeriveKey(password string, salt []byte) []byte {
	// TODO: Implement Argon2 key derivation
	return nil
}

// EncryptWithPassword encrypts data using a password
// Handles key derivation and salt generation internally
func EncryptWithPassword(plaintext, password string) (string, error) {
	// TODO: Implement password-based encryption
	return "", errors.New("not implemented")
}

// DecryptWithPassword decrypts data encrypted with password
func DecryptWithPassword(ciphertext, password string) (string, error) {
	// TODO: Implement password-based decryption
	return "", errors.New("not implemented")
}
