package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/argon2"
)

const (
	KeySize   = 32 // AES-256 requires 32 bytes
	NonceSize = 12 // GCM standard nonce size
	SaltSize  = 16 // Salt for key derivation
)

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns base64-encoded ciphertext with embedded nonce.
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
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	// 5. Encrypt data
	ciphertext := aesGcm.Seal(nil, nonce, []byte(plaintext), nil)

	// 6. Prepend nonce to ciphertext
	result := append(nonce, ciphertext...)

	// 7. Encode as base64
	return base64.StdEncoding.EncodeToString(result), nil
}

// Decrypt decrypts AES-256-GCM encrypted data.
// Expects base64-encoded ciphertext with embedded nonce.
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

// GenerateKey generates a cryptographically secure random 32-byte encryption key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// DeriveKey derives an encryption key from a password using Argon2id.
// Uses hardcoded parameters: time=1, memory=64MB, threads=4.
// TODO: Make Argon2 parameters configurable for different security requirements.
func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, KeySize)
}

// EncryptWithPassword encrypts data using a password-derived key.
// Handles salt generation and key derivation internally.
// Returns base64-encoded ciphertext with embedded salt and nonce.
func EncryptWithPassword(plaintext, password string) (string, error) {
	// 1. Generate random salt
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// 2. Derive key from password + salt
	key := DeriveKey(password, salt)

	// 3. Encrypt data using the derived key
	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		return "", err
	}

	// 4. Decode the encrypted data to get raw bytes
	encryptedBytes, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	// 5. Prepend salt to the encrypted data (salt + nonce + ciphertext)
	result := append(salt, encryptedBytes...)

	// 6. Encode everything as base64
	return base64.StdEncoding.EncodeToString(result), nil
}

// DecryptWithPassword decrypts data encrypted with password
// Expects base64-encoded: salt + nonce + ciphertext
func DecryptWithPassword(ciphertext, password string) (string, error) {
	// 1. Decode from base64
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	// 2. Check minimum length (salt + nonce + some data)
	if len(decoded) < SaltSize+NonceSize {
		return "", errors.New("ciphertext too short")
	}

	// 3. Extract salt (first 16 bytes)
	salt := decoded[:SaltSize]

	// 4. Extract remaining data (nonce + ciphertext)
	encryptedData := decoded[SaltSize:]

	// 5. Derive key from password + salt
	key := DeriveKey(password, salt)

	// 6. Encode the encrypted data back to base64 for Decrypt function
	encryptedString := base64.StdEncoding.EncodeToString(encryptedData)

	// 7. Decrypt using the derived key
	plaintext, err := Decrypt(encryptedString, key)
	if err != nil {
		return "", err
	}

	return plaintext, nil
}
