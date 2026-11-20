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
	for _, kept := range []string{"a@example.com", `"limit":20`} {
		if !strings.Contains(out, kept) {
			t.Fatalf("non-secret %q was lost: %s", kept, out)
		}
	}
}

// DM・投稿の本文、自己紹介、表示名、検索語は利用者が書いた文章なので残さない。
// 本番の監査ログに平文で出ていた項目名をそのまま使う。
func TestMaskVariablesHidesUserWrittenText(t *testing.T) {
	var vars map[string]any
	body := `{
		"roomID": "cm9vbTo5Mw", "content": "dm-body", "replyToID": null, "mentionUserIDs": ["dXNlcjoyNDQ"],
		"input": {"content": "post-body", "post_id": "p1", "bio": "my-bio", "name": "my-name", "accountID": "acct1",
			"mediaInputs": [{"contentType": "image/png", "objectKey": "k/1.png", "width": 10, "height": 20, "caption": "cap-text"}],
			"pageViews": [{"path": "/dm/abc", "durationSeconds": 3, "resumed": true}]},
		"keyword": "search-word", "tag": "tag-word", "filter": {"status": "OPEN"},
		"since": "2026-11-20T00:00:00Z", "sortBy": "CREATED_AT", "offset": 0
	}`
	if err := json.Unmarshal([]byte(body), &vars); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(maskVariables(vars))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, text := range []string{"dm-body", "post-body", "my-bio", "my-name", "cap-text", "search-word", "tag-word"} {
		if strings.Contains(out, text) {
			t.Fatalf("user text %q left in log output: %s", text, out)
		}
	}
	for _, kept := range []string{"cm9vbTo5Mw", "dXNlcjoyNDQ", `"post_id":"p1"`, "acct1", "image/png", "k/1.png", `"width":10`,
		"/dm/abc", `"durationSeconds":3`, `"resumed":true`, `"replyToID":null`, "OPEN", "2026-11-20T00:00:00Z", "CREATED_AT", `"offset":0`} {
		if !strings.Contains(out, kept) {
			t.Fatalf("investigation value %q was lost: %s", kept, out)
		}
	}
}
