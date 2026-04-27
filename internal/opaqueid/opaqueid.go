package opaqueid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultSecret = "space-default-opaque-id-secret"

func secret() []byte {
	if v := strings.TrimSpace(os.Getenv("OPAQUE_ID_SECRET")); v != "" {
		return []byte(v)
	}
	return []byte(defaultSecret)
}

func Encode(kind string, id int64) string {
	payload := fmt.Sprintf("%s:%d", kind, id)
	mac := sign(payload)
	token := payload + ":" + mac
	return base64.RawURLEncoding.EncodeToString([]byte(token))
}

func Decode(kind, opaque string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(opaque)
	if err != nil {
		return 0, errors.New("invalid opaque id")
	}

	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 {
		return 0, errors.New("invalid opaque id")
	}

	if parts[0] != kind {
		return 0, errors.New("invalid opaque id kind")
	}

	payload := parts[0] + ":" + parts[1]
	expected := sign(payload)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return 0, errors.New("invalid opaque id signature")
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, errors.New("invalid opaque id value")
	}

	return id, nil
}

func sign(payload string) string {
	h := hmac.New(sha256.New, secret())
	h.Write([]byte(payload))
	sum := h.Sum(nil)
	// 先頭12byteで十分な識別性を確保しつつトークン長を抑える。
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}
