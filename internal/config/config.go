// Package config validates required environment configuration at startup so a
// missing or insecure setting fails fast with a clear, aggregated error instead
// of surfacing later as a runtime failure the first time a feature is used.
package config

import (
	"fmt"
	"os"
	"strings"
)

// defaultOpaqueIDSecret mirrors the fallback in internal/opaqueid. Running with
// it in production means opaque IDs are signed with a publicly known key, so we
// reject it here.
const defaultOpaqueIDSecret = "space-default-opaque-id-secret"

const localActivityArchiveHMACKey = "local-dev-activity-archive-hmac-key"

// ActivityArchiveHMACKey keeps local startup usable without weakening production.
func ActivityArchiveHMACKey(isProd bool) string {
	key := os.Getenv("ACTIVITY_ARCHIVE_HMAC_KEY")
	if key == "" && !isProd {
		return localActivityArchiveHMACKey
	}
	return key
}

// Validate checks that all configuration required to run safely is present.
// In production it enforces secrets and infrastructure endpoints; in non-prod it
// only warns, so local development keeps working with defaults.
// It returns a single error aggregating every problem found.
func Validate(isProd bool) error {
	var problems []string

	require := func(key string) {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			problems = append(problems, fmt.Sprintf("%s must be set", key))
		}
	}

	// Always required, in every environment.
	for _, key := range []string{"DB_USER", "DB_PASSWORD", "DB_HOST", "DB_NAME"} {
		require(key)
	}

	// JWT signing secret: without it, no token can be issued or verified.
	require("JWT_SECRET")

	if isProd {
		// Production-only hardening.
		require("ALLOWED_ORIGINS")
		if len(ActivityArchiveHMACKey(true)) < 32 || ActivityArchiveHMACKey(true) == localActivityArchiveHMACKey {
			problems = append(problems, "ACTIVITY_ARCHIVE_HMAC_KEY must be a non-default secret of at least 32 bytes in production")
		}

		if secret := strings.TrimSpace(os.Getenv("OPAQUE_ID_SECRET")); secret == "" || secret == defaultOpaqueIDSecret {
			problems = append(problems, "OPAQUE_ID_SECRET must be set to a non-default value in production")
		}

		// Object storage: the selected provider's credentials must be present.
		if os.Getenv("STORAGE_PROVIDER") == "azure" {
			for _, key := range []string{"AZURE_STORAGE_ACCOUNT_NAME", "AZURE_STORAGE_CONTAINER_NAME"} {
				require(key)
			}

			// 署名する手段は2つあり、どちらか片方で足りる（infra/azure の
			// AzureBlobStorageRepository を参照）。
			//
			//   1. AZURE_STORAGE_ACCOUNT_KEY（アカウントキー）
			//   2. Entra ID の ID: アプリ登録なら AZURE_TENANT_ID /
			//      AZURE_CLIENT_ID / AZURE_CLIENT_SECRET の3点、Azure 上の
			//      マネージド ID なら環境変数は要らない
			//
			// マネージド ID は env に痕跡を残さないので「何も無い」を誤りとは
			// 判定できない。代わりに3点の「途中まで」を弾く。実際に起きる誤りは
			// 1つ入れ忘れる形で、これは起動時の委任キー取得が Azure 側の
			// 認証エラーとして返すため原因が読み取りにくい。
			if strings.TrimSpace(os.Getenv("AZURE_STORAGE_ACCOUNT_KEY")) == "" {
				appRegistration := []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"}
				var missing []string
				for _, key := range appRegistration {
					if strings.TrimSpace(os.Getenv(key)) == "" {
						missing = append(missing, key)
					}
				}
				if len(missing) > 0 && len(missing) < len(appRegistration) {
					problems = append(problems, fmt.Sprintf(
						"%s must be set when authenticating with an app registration (or set AZURE_STORAGE_ACCOUNT_KEY instead)",
						strings.Join(missing, ", ")))
				}
			}
		} else {
			for _, key := range []string{"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET"} {
				require(key)
			}
		}

		// Redis backs token revocation, OTPs, password reset and maintenance state.
		require("REDIS_HOST")

		// Outbound mail (OTP, password reset).
		for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_FROM"} {
			require(key)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
}
