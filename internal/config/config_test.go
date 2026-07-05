package config

import (
	"strings"
	"testing"
)

// setEnv sets each key for the duration of the test via t.Setenv (auto-restored).
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

var validProdEnv = map[string]string{
	"DB_USER":          "u",
	"DB_PASSWORD":      "p",
	"DB_HOST":          "h",
	"DB_NAME":          "n",
	"JWT_SECRET":       "s",
	"ALLOWED_ORIGINS":  "https://example.com",
	"OPAQUE_ID_SECRET": "a-real-secret",
	"MINIO_ENDPOINT":   "e",
	"MINIO_ACCESS_KEY": "a",
	"MINIO_SECRET_KEY": "s",
	"MINIO_BUCKET":     "b",
	"REDIS_HOST":       "r",
	"SMTP_HOST":        "sh",
	"SMTP_PORT":        "587",
	"SMTP_FROM":        "no-reply@example.com",
}

func TestValidate_DevOnlyNeedsDBAndJWT(t *testing.T) {
	setEnv(t, map[string]string{
		"DB_USER": "u", "DB_PASSWORD": "p", "DB_HOST": "h", "DB_NAME": "n", "JWT_SECRET": "s",
	})
	if err := Validate(false); err != nil {
		t.Fatalf("dev config with DB + JWT should pass, got: %v", err)
	}
}

func TestValidate_DevFailsWithoutJWT(t *testing.T) {
	setEnv(t, map[string]string{"DB_USER": "u", "DB_PASSWORD": "p", "DB_HOST": "h", "DB_NAME": "n"})
	t.Setenv("JWT_SECRET", "")
	if err := Validate(false); err == nil {
		t.Fatal("expected error when JWT_SECRET missing")
	}
}

func TestValidate_ProdPassesWithFullConfig(t *testing.T) {
	setEnv(t, validProdEnv)
	if err := Validate(true); err != nil {
		t.Fatalf("complete prod config should pass, got: %v", err)
	}
}

func TestValidate_ProdRejectsDefaultOpaqueSecret(t *testing.T) {
	setEnv(t, validProdEnv)
	t.Setenv("OPAQUE_ID_SECRET", defaultOpaqueIDSecret)
	err := Validate(true)
	if err == nil || !strings.Contains(err.Error(), "OPAQUE_ID_SECRET") {
		t.Fatalf("expected OPAQUE_ID_SECRET rejection, got: %v", err)
	}
}

func TestValidate_ProdAggregatesMultipleProblems(t *testing.T) {
	setEnv(t, validProdEnv)
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("REDIS_HOST", "")
	err := Validate(true)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "ALLOWED_ORIGINS") || !strings.Contains(err.Error(), "REDIS_HOST") {
		t.Errorf("error should mention every missing key, got: %v", err)
	}
}
