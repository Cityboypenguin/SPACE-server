package messagecrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const Prefix = "space-msg-v1:"

type Cipher struct {
	aead cipher.AEAD
}

func New(key string) (*Cipher, error) {
	keyBytes, err := decodeKey(strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("create message cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create message gcm: %w", err)
	}

	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate message nonce: %w", err)
	}

	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return Prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *Cipher) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, Prefix) {
		return value, nil
	}

	payload := strings.TrimPrefix(value, Prefix)
	sealed, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("decode encrypted message: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(sealed) < nonceSize {
		return "", errors.New("encrypted message payload is too short")
	}

	nonce := sealed[:nonceSize]
	ciphertext := sealed[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt message: %w", err)
	}

	return string(plaintext), nil
}

func decodeKey(key string) ([]byte, error) {
	if key == "" {
		return nil, errors.New("MESSAGE_ENCRYPTION_KEY environment variable must be set")
	}
	if len(key) == 32 {
		return []byte(key), nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}

	return nil, errors.New("MESSAGE_ENCRYPTION_KEY must decode to 32 bytes")
}
