package crypto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncryptDecrypt tests basic encryption and decryption
func TestEncryptDecrypt(t *testing.T) {
	// Generate a random key
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i) // Simple test key
	}

	testCases := []struct {
		name      string
		plaintext string
	}{
		{"Simple text", "hello world"},
		{"Empty string", ""},
		{"Long text", strings.Repeat("a", 1000)},
		{"Special chars", "!@#$%^&*()_+-={}[]|\\:;\"'<>,.?/"},
		{"Bitcoin key", "L3fKPqKvGPZxVvGFm8YqXb7kNmXvHwgPqR2rRnVdKLqX9Yt3Qw2M"},
		{"Unicode", "Hello 世界 🌍"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := Encrypt(tc.plaintext, key)
			require.NoError(t, err, "Encryption should succeed")
			assert.NotEmpty(t, encrypted, "Encrypted text should not be empty")
			assert.NotEqual(t, encrypted, tc.plaintext, "Encrypted text should differ from plaintext")

			// Decrypt
			decrypted, err := Decrypt(encrypted, key)
			require.NoError(t, err, "Decryption should succeed")

			assert.Equal(t, tc.plaintext, decrypted, "Decrypted text should match original plaintext")
		})
	}
}

// TestEncryptDifferentOutputs tests that same plaintext produces different ciphertexts
func TestEncryptDifferentOutputs(t *testing.T) {
	key := make([]byte, KeySize)
	plaintext := "same plaintext"

	// Encrypt same text multiple times
	encrypted1, _ := Encrypt(plaintext, key)
	encrypted2, _ := Encrypt(plaintext, key)
	encrypted3, _ := Encrypt(plaintext, key)

	// All should be different (due to random nonce)
	assert.NotEqual(t, encrypted1, encrypted2, "Multiple encryptions should produce different ciphertexts")
	assert.NotEqual(t, encrypted1, encrypted3, "Multiple encryptions should produce different ciphertexts")
	assert.NotEqual(t, encrypted2, encrypted3, "Multiple encryptions should produce different ciphertexts")

	// But all should decrypt to same plaintext
	dec1, _ := Decrypt(encrypted1, key)
	dec2, _ := Decrypt(encrypted2, key)
	dec3, _ := Decrypt(encrypted3, key)

	if dec1 != plaintext || dec2 != plaintext || dec3 != plaintext {
		t.Fatal("Decryption failed for multiple encryptions")
	}
}

// TestEncryptWithWrongKey tests decryption with wrong key fails
func TestDecryptWithWrongKey(t *testing.T) {
	key1 := make([]byte, KeySize)
	key2 := make([]byte, KeySize)
	key2[0] = 1 // Different key

	plaintext := "secret message"

	// Encrypt with key1
	encrypted, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Try to decrypt with key2 (wrong key)
	_, err = Decrypt(encrypted, key2)
	if err == nil {
		t.Fatal("Decryption should fail with wrong key")
	}

	// Error message should indicate failure
	if !strings.Contains(err.Error(), "decryption failed") {
		t.Fatalf("Expected 'decryption failed' error, got: %v", err)
	}
}

// TestEncryptWithInvalidKey tests encryption with invalid key size
func TestEncryptWithInvalidKey(t *testing.T) {
	testCases := []struct {
		name    string
		keySize int
	}{
		{"Too short", 16},
		{"Too long", 64},
		{"Empty", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			invalidKey := make([]byte, tc.keySize)
			_, err := Encrypt("test", invalidKey)
			if err == nil {
				t.Fatal("Encryption should fail with invalid key size")
			}
			if !strings.Contains(err.Error(), "32 bytes") {
				t.Fatalf("Error should mention 32 bytes, got: %v", err)
			}
		})
	}
}

// TestDecryptWithInvalidKey tests decryption with invalid key size
func TestDecryptWithInvalidKey(t *testing.T) {
	invalidKey := make([]byte, 16) // Wrong size
	_, err := Decrypt("someencrypteddata", invalidKey)
	if err == nil {
		t.Fatal("Decryption should fail with invalid key size")
	}
}

// TestDecryptWithInvalidData tests decryption with corrupted data
func TestDecryptWithInvalidData(t *testing.T) {
	key := make([]byte, KeySize)

	testCases := []struct {
		name       string
		ciphertext string
	}{
		{"Invalid base64", "not-valid-base64!!!"},
		{"Too short", "YWJj"}, // "abc" in base64 - only 3 bytes
		{"Empty", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decrypt(tc.ciphertext, key)
			if err == nil {
				t.Fatal("Decryption should fail with invalid data")
			}
		})
	}
}

// TestDecryptWithTamperedData tests that tampering is detected
func TestDecryptWithTamperedData(t *testing.T) {
	key := make([]byte, KeySize)
	plaintext := "original message"

	// Encrypt
	encrypted, _ := Encrypt(plaintext, key)

	// Tamper with encrypted data (flip one character)
	tamperedBytes := []byte(encrypted)
	if tamperedBytes[10] == 'A' {
		tamperedBytes[10] = 'B'
	} else {
		tamperedBytes[10] = 'A'
	}
	tampered := string(tamperedBytes)

	// Try to decrypt tampered data
	_, err := Decrypt(tampered, key)
	if err == nil {
		t.Fatal("Decryption should fail with tampered data (GCM authentication should catch this)")
	}
}

// TestLongPlaintext tests encryption of large data
func TestLongPlaintext(t *testing.T) {
	key := make([]byte, KeySize)

	// Test with 1MB of data
	plaintext := strings.Repeat("a", 1024*1024)

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encryption failed for large data: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decryption failed for large data: %v", err)
	}

	if decrypted != plaintext {
		t.Fatal("Large data decryption mismatch")
	}
}

// BenchmarkEncrypt benchmarks encryption performance
func BenchmarkEncrypt(b *testing.B) {
	key := make([]byte, KeySize)
	plaintext := "benchmark test message"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Encrypt(plaintext, key)
	}
}

// BenchmarkDecrypt benchmarks decryption performance
func BenchmarkDecrypt(b *testing.B) {
	key := make([]byte, KeySize)
	plaintext := "benchmark test message"
	encrypted, _ := Encrypt(plaintext, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decrypt(encrypted, key)
	}
}

// TestEncryptDecryptWithPassword tests password-based encryption
func TestEncryptDecryptWithPassword(t *testing.T) {
	testCases := []struct {
		name      string
		plaintext string
		password  string
	}{
		{"Simple", "hello world", "mypassword123"},
		{"Empty plaintext", "", "password"},
		{"Long password", "secret", "this-is-a-very-long-password-with-special-chars-!@#$%"},
		{"Unicode", "Hello 世界", "パスワード"},
		{"Bitcoin key", "L3fKPqKvGPZxVvGFm8YqXb7kNmXvHwgPqR2rRnVdKLqX9Yt3Qw2M", "securepass"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt with password
			encrypted, err := EncryptWithPassword(tc.plaintext, tc.password)
			if err != nil {
				t.Fatalf("EncryptWithPassword failed: %v", err)
			}

			// Verify not empty
			if encrypted == "" {
				t.Fatal("Encrypted result is empty")
			}

			// Decrypt with same password
			decrypted, err := DecryptWithPassword(encrypted, tc.password)
			if err != nil {
				t.Fatalf("DecryptWithPassword failed: %v", err)
			}

			// Verify match
			if decrypted != tc.plaintext {
				t.Fatalf("Decrypted doesn't match. Expected: %q, Got: %q", tc.plaintext, decrypted)
			}
		})
	}
}

// TestPasswordEncryptionDifferentOutputs tests different salts produce different outputs
func TestPasswordEncryptionDifferentOutputs(t *testing.T) {
	plaintext := "same text"
	password := "same password"

	// Encrypt multiple times
	enc1, _ := EncryptWithPassword(plaintext, password)
	enc2, _ := EncryptWithPassword(plaintext, password)
	enc3, _ := EncryptWithPassword(plaintext, password)

	// Should all be different (different salts + nonces)
	if enc1 == enc2 || enc1 == enc3 || enc2 == enc3 {
		t.Fatal("Multiple encryptions produced identical output (salt/nonce reuse!)")
	}

	// But all should decrypt correctly
	dec1, _ := DecryptWithPassword(enc1, password)
	dec2, _ := DecryptWithPassword(enc2, password)
	dec3, _ := DecryptWithPassword(enc3, password)

	if dec1 != plaintext || dec2 != plaintext || dec3 != plaintext {
		t.Fatal("Not all encryptions decrypted correctly")
	}
}

// TestDecryptWithPasswordWrongPassword tests wrong password fails
func TestDecryptWithPasswordWrongPassword(t *testing.T) {
	plaintext := "secret message"
	password := "correct-password"
	wrongPassword := "wrong-password"

	// Encrypt
	encrypted, err := EncryptWithPassword(plaintext, password)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Try to decrypt with wrong password
	_, err = DecryptWithPassword(encrypted, wrongPassword)
	if err == nil {
		t.Fatal("Decryption should fail with wrong password")
	}

	if !strings.Contains(err.Error(), "decryption failed") {
		t.Fatalf("Expected 'decryption failed' error, got: %v", err)
	}
}

// TestPasswordEncryptionEmptyPassword tests encryption with empty password
func TestPasswordEncryptionEmptyPassword(t *testing.T) {
	plaintext := "test data"
	password := ""

	// Should still work (though not recommended)
	encrypted, err := EncryptWithPassword(plaintext, password)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	decrypted, err := DecryptWithPassword(encrypted, password)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatal("Decryption with empty password failed")
	}
}

// TestDeriveKey tests key derivation produces consistent results
func TestDeriveKey(t *testing.T) {
	password := "mypassword"
	salt := []byte("1234567890123456") // 16 bytes

	// Derive key twice with same inputs
	key1 := DeriveKey(password, salt)
	key2 := DeriveKey(password, salt)

	// Should be identical
	if string(key1) != string(key2) {
		t.Fatal("DeriveKey produced different keys for same input")
	}

	// Should be correct length
	if len(key1) != KeySize {
		t.Fatalf("DeriveKey produced wrong key size. Expected %d, got %d", KeySize, len(key1))
	}

	// Different salt should produce different key
	differentSalt := []byte("9876543210987654")
	key3 := DeriveKey(password, differentSalt)

	if string(key1) == string(key3) {
		t.Fatal("Different salts produced same key")
	}
}

// TestGenerateKey tests random key generation
func TestGenerateKey(t *testing.T) {
	// Generate multiple keys
	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	// Should be correct size
	if len(key1) != KeySize || len(key2) != KeySize {
		t.Fatal("Generated keys are not correct size")
	}

	// Should be different
	if string(key1) == string(key2) {
		t.Fatal("Generated keys are identical (bad randomness!)")
	}
}
