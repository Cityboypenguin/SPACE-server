package azurerepo

import (
	"os"
	"testing"
)

// 空文字の変数は消えること。中身のある変数は残ること。
//
// compose は値を与えていない変数も空文字で渡す。空のまま SDK に渡すと
// 「利用者割り当て ID を指定された（中身は空）」と解釈され、マネージド ID で
// 動く構成が起動しなくなる。
func TestClearEmptyCredentialEnvRemovesOnlyEmptyOnes(t *testing.T) {
	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_TENANT_ID", "   ")
	t.Setenv("AZURE_CLIENT_SECRET", "ほんとうの値")

	clearEmptyCredentialEnv()

	if _, ok := os.LookupEnv("AZURE_CLIENT_ID"); ok {
		t.Error("空の AZURE_CLIENT_ID が残っている")
	}
	if _, ok := os.LookupEnv("AZURE_TENANT_ID"); ok {
		t.Error("空白だけの AZURE_TENANT_ID が残っている")
	}
	if got := os.Getenv("AZURE_CLIENT_SECRET"); got != "ほんとうの値" {
		t.Errorf("中身のある変数が %q になった（消してはいけない）", got)
	}
}

// 変数が最初から無い場合に、新しく作ってしまわないこと。
func TestClearEmptyCredentialEnvDoesNotCreateVars(t *testing.T) {
	for _, name := range credentialEnvVars {
		t.Setenv(name, "x")
		os.Unsetenv(name)
	}

	clearEmptyCredentialEnv()

	for _, name := range credentialEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s が作られている", name)
		}
	}
}
