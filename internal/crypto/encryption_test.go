package crypto

import (
	"strings"
	"testing"
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
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}

			// Verify encrypted is not empty
			if encrypted == "" {
				t.Fatal("Encrypted text is empty")
			}

			// Verify encrypted is different from plaintext
			if encrypted == tc.plaintext {
				t.Fatal("Encrypted text is same as plaintext")
			}

			// Decrypt
			decrypted, err := Decrypt(encrypted, key)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}

			// Verify decrypted matches original
			if decrypted != tc.plaintext {
				t.Fatalf("Decrypted text doesn't match. Expected: %q, Got: %q", tc.plaintext, decrypted)
			}
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
	if encrypted1 == encrypted2 || encrypted1 == encrypted3 || encrypted2 == encrypted3 {
		t.Fatal("Multiple encryptions of same text produced identical ciphertexts (nonce reuse!)")
	}

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
