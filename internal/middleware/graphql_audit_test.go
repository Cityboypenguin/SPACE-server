package middleware

import (
	"encoding/json"
	"strings"
	"testing"
)

// ログインと登録は variables.input の下にパスワードやワンタイムパスワードを入れて送る。
// 実際に本番の監査ログへ平文で出ていた形をそのまま使う。
func TestMaskVariablesMasksNestedSecrets(t *testing.T) {
	var vars map[string]any
	body := `{
		"input": {"email": "a@example.com", "password": "plain-pass", "otp": "123456", "name": "学生"},
		"email": "a@example.com", "otp": "654321",
		"items": [{"newPassword": "plain-new"}, {"refreshToken": "plain-token"}],
		"limit": 20
	}`
	if err := json.Unmarshal([]byte(body), &vars); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(maskVariables(vars))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, secret := range []string{"plain-pass", "123456", "654321", "plain-new", "plain-token"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret %q left in log output: %s", secret, out)
		}
	}
	// 調査に要る値は残す。
	for _, kept := range []string{"a@example.com", "学生", `"limit":20`} {
		if !strings.Contains(out, kept) {
			t.Fatalf("non-secret %q was lost: %s", kept, out)
		}
	}
}
