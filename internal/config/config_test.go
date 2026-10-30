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
	"DB_USER":                   "u",
	"DB_PASSWORD":               "p",
	"DB_HOST":                   "h",
	"DB_NAME":                   "n",
	"JWT_SECRET":                "s",
	"ALLOWED_ORIGINS":           "https://example.com",
	"OPAQUE_ID_SECRET":          "a-real-secret",
	"MINIO_ENDPOINT":            "e",
	"MINIO_ACCESS_KEY":          "a",
	"MINIO_SECRET_KEY":          "s",
	"MINIO_BUCKET":              "b",
	"REDIS_HOST":                "r",
	"SMTP_HOST":                 "sh",
	"SMTP_PORT":                 "587",
	"SMTP_FROM":                 "no-reply@example.com",
	"ACTIVITY_ARCHIVE_HMAC_KEY": "production-activity-archive-hmac-secret",
}

func TestActivityArchiveHMACKey_DevFallback(t *testing.T) {
	t.Setenv("ACTIVITY_ARCHIVE_HMAC_KEY", "")
	if got := ActivityArchiveHMACKey(false); got != localActivityArchiveHMACKey {
		t.Fatalf("dev key = %q, want local fallback", got)
	}
	if got := ActivityArchiveHMACKey(true); got != "" {
		t.Fatalf("production key = %q, want no fallback", got)
	}
}

func TestValidate_ProdRequiresDedicatedActivityArchiveKey(t *testing.T) {
	setEnv(t, validProdEnv)
	for _, key := range []string{"", "short", localActivityArchiveHMACKey} {
		t.Setenv("ACTIVITY_ARCHIVE_HMAC_KEY", key)
		if err := Validate(true); err == nil || !strings.Contains(err.Error(), "ACTIVITY_ARCHIVE_HMAC_KEY") {
			t.Fatalf("key %q: expected validation error, got %v", key, err)
		}
	}
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

// azureProdEnv は Azure を選んだ本番の土台。資格情報はテストごとに足す。
func azureProdEnv() map[string]string {
	env := map[string]string{}
	for k, v := range validProdEnv {
		env[k] = v
	}
	env["STORAGE_PROVIDER"] = "azure"
	env["AZURE_STORAGE_ACCOUNT_NAME"] = "acct"
	env["AZURE_STORAGE_CONTAINER_NAME"] = "space-avatars"
	return env
}

// アカウントキー方式（従来）。
func TestValidate_AzureAcceptsAccountKey(t *testing.T) {
	env := azureProdEnv()
	env["AZURE_STORAGE_ACCOUNT_KEY"] = "k"
	setEnv(t, env)
	if err := Validate(true); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// アプリ登録方式。アカウントキーが無くても3点が揃っていれば通ること。
func TestValidate_AzureAcceptsAppRegistration(t *testing.T) {
	env := azureProdEnv()
	env["AZURE_TENANT_ID"] = "t"
	env["AZURE_CLIENT_ID"] = "c"
	env["AZURE_CLIENT_SECRET"] = "s"
	setEnv(t, env)
	if err := Validate(true); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// マネージド ID は環境変数に痕跡を残さない。「何も無い」を誤りと決めつけると
// その構成で起動できなくなるので、通すこと。実際の資格情報は
// infra/azure の New が起動時に確かめる。
func TestValidate_AzureAcceptsManagedIdentity(t *testing.T) {
	setEnv(t, azureProdEnv())
	if err := Validate(true); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// 3点のうち1つを入れ忘れた場合。ここを通すと、起動時の委任キー取得が
// Azure 側の認証エラーになり、原因が読み取りにくい形で出る。
func TestValidate_AzureRejectsPartialAppRegistration(t *testing.T) {
	env := azureProdEnv()
	env["AZURE_TENANT_ID"] = "t"
	env["AZURE_CLIENT_ID"] = "c"
	setEnv(t, env)

	err := Validate(true)
	if err == nil {
		t.Fatal("Validate() = nil, want an error about the missing secret")
	}
	if !strings.Contains(err.Error(), "AZURE_CLIENT_SECRET") {
		t.Errorf("error = %q, want it to name AZURE_CLIENT_SECRET", err)
	}
}

// コンテナー名とアカウント名はどちらの方式でも要る。
func TestValidate_AzureRequiresAccountAndContainer(t *testing.T) {
	env := azureProdEnv()
	env["AZURE_STORAGE_ACCOUNT_NAME"] = ""
	env["AZURE_STORAGE_CONTAINER_NAME"] = ""
	env["AZURE_STORAGE_ACCOUNT_KEY"] = "k"
	setEnv(t, env)

	err := Validate(true)
	if err == nil {
		t.Fatal("Validate() = nil, want errors about the account and container")
	}
	for _, want := range []string{"AZURE_STORAGE_ACCOUNT_NAME", "AZURE_STORAGE_CONTAINER_NAME"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %s", err, want)
		}
	}
}
