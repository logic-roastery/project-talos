package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := GenerateKey()

	ciphertext, err := Encrypt("hello world", key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	plaintext, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if plaintext != "hello world" {
		t.Fatalf("Decrypt returned %q, want %q", plaintext, "hello world")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := GenerateKey()
	key2 := GenerateKey()

	ciphertext, err := Encrypt("hello world", key1)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Fatal("Decrypt with wrong key should have returned error")
	}
}

func TestDecryptCiphertextTooShort(t *testing.T) {
	key := GenerateKey()

	short := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := Decrypt(short, key)
	if err == nil {
		t.Fatal("Decrypt with short ciphertext should have returned error")
	}
	if !strings.Contains(err.Error(), "ciphertext too short") {
		t.Fatalf("Decrypt returned error %q, want error containing %q", err, "ciphertext too short")
	}
}

func TestEncryptInvalidKeyLength(t *testing.T) {
	key := make([]byte, 16)
	_, err := Encrypt("hello", key)
	if err == nil {
		t.Fatal("Encrypt with 16-byte key should have returned error")
	}
}

func TestDecryptInvalidKeyLength(t *testing.T) {
	key := make([]byte, 16)
	_, err := Decrypt("anything", key)
	if err == nil {
		t.Fatal("Decrypt with 16-byte key should have returned error")
	}
}

func TestEncodeDecodeKey(t *testing.T) {
	key := GenerateKey()
	encoded := EncodeKey(key)

	decoded, err := DecodeKey(encoded)
	if err != nil {
		t.Fatalf("DecodeKey returned error: %v", err)
	}
	if len(decoded) != len(key) {
		t.Fatalf("DecodeKey returned key of length %d, want %d", len(decoded), len(key))
	}
	for i := range key {
		if key[i] != decoded[i] {
			t.Fatalf("DecodeKey returned key that differs at byte %d", i)
		}
	}
}

func TestDecodeKeyInvalidBase64(t *testing.T) {
	_, err := DecodeKey("not-base64!!!")
	if err == nil {
		t.Fatal("DecodeKey with invalid base64 should have returned error")
	}
}

func TestDecodeKeyWrongLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := DecodeKey(short)
	if err == nil {
		t.Fatal("DecodeKey with 16-byte key should have returned error")
	}
	if !strings.Contains(err.Error(), "must be 32 bytes") {
		t.Fatalf("DecodeKey returned error %q, want error containing %q", err, "must be 32 bytes")
	}
}
