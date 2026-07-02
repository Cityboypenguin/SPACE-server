package messagecrypto

import (
	"strings"
	"testing"
)

func TestCipherEncryptDecrypt(t *testing.T) {
	cipher, err := New("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	encrypted, err := cipher.Encrypt("hello secret message")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if !strings.HasPrefix(encrypted, Prefix) {
		t.Fatalf("encrypted value does not have prefix: %q", encrypted)
	}
	if strings.Contains(encrypted, "hello secret message") {
		t.Fatalf("encrypted value contains plaintext: %q", encrypted)
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != "hello secret message" {
		t.Fatalf("Decrypt() = %q", decrypted)
	}
}

func TestCipherDecryptPlaintextCompatibility(t *testing.T) {
	cipher, err := New("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	decrypted, err := cipher.Decrypt("legacy plaintext")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != "legacy plaintext" {
		t.Fatalf("Decrypt() = %q", decrypted)
	}
}

func TestCipherRequires32ByteKey(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Fatal("New() error = nil")
	}
}
