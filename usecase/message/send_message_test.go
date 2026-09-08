package message

import (
	"context"
	"strings"
	"testing"
	"time"
)

func newSendMessageUseCase(repo *fakeMessageRepo) SendMessageUseCase {
	return NewSendMessageUseCase(repo, &fakeMediaRepo{}, fakeTxManager{})
}

// 項番2: 空文字を送信 -> 送信できない
func TestSendMessage_RejectsEmptyContent(t *testing.T) {
	uc := newSendMessageUseCase(newFakeMessageRepo())
	if _, err := uc.Execute(context.Background(), 1, 7, "", nil); err == nil {
		t.Fatal("expected error for empty content")
	}
}

// 項番3: スペースのみ送信 -> 送信できない（trim後に空文字と判定される）
func TestSendMessage_RejectsWhitespaceOnlyContent(t *testing.T) {
	uc := newSendMessageUseCase(newFakeMessageRepo())
	if _, err := uc.Execute(context.Background(), 1, 7, "   \t\n  ", nil); err == nil {
		t.Fatal("expected error for whitespace-only content")
	}
}

// 項番4: 最大文字数ちょうど -> 正常に送信できる
func TestSendMessage_AcceptsContentAtMaxLength(t *testing.T) {
	repo := newFakeMessageRepo()
	uc := newSendMessageUseCase(repo)
	content := strings.Repeat("あ", MaxContentChars)

	msg, err := uc.Execute(context.Background(), 1, 7, content, nil)
	if err != nil {
		t.Fatalf("unexpected error at exactly max length: %v", err)
	}
	if msg.Content != content {
		t.Errorf("stored content was altered at max length boundary")
	}
}

// 項番5: 最大文字数+1文字 -> 送信できず、DBにも保存されない
func TestSendMessage_RejectsContentOverMaxLength(t *testing.T) {
	repo := newFakeMessageRepo()
	uc := newSendMessageUseCase(repo)
	content := strings.Repeat("あ", MaxContentChars+1)

	if _, err := uc.Execute(context.Background(), 1, 7, content, nil); err == nil {
		t.Fatal("expected error for content exceeding max length")
	}
	if len(repo.messages) != 0 {
		t.Fatalf("message must not be persisted when validation fails, got %d rows", len(repo.messages))
	}
}

// 項番6-10: 絵文字・改行・HTMLタグ・XSS文字列・SQL風文字列は加工されず、
// そのままの文字列としてリポジトリに渡される（サーバー側でエスケープ/実行/解釈しない）。
func TestSendMessage_StoresSpecialContentVerbatim(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"emoji", "😀👍"},
		{"newline", "1行目\n2行目"},
		{"html_tag", "<b>test</b>"},
		{"xss_script", "<script>alert(1)</script>"},
		{"sql_like", "' OR '1'='1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeMessageRepo()
			uc := newSendMessageUseCase(repo)

			msg, err := uc.Execute(context.Background(), 1, 7, tc.content, nil)
			if err != nil {
				t.Fatalf("unexpected error sending %q: %v", tc.content, err)
			}
			if msg.Content != tc.content {
				t.Errorf("content was altered: got %q, want %q", msg.Content, tc.content)
			}
			stored := repo.getMessageByIDIncludingDeleted(msg.ID)
			if stored == nil || stored.Content != tc.content {
				t.Errorf("persisted content = %q, want %q", stored.Content, tc.content)
			}
		})
	}
}

// 項番43: 前後の空白は trim されて保存される
func TestSendMessage_TrimsSurroundingWhitespace(t *testing.T) {
	repo := newFakeMessageRepo()
	uc := newSendMessageUseCase(repo)

	msg, err := uc.Execute(context.Background(), 1, 7, "  こんにちは  ", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "こんにちは" {
		t.Errorf("content = %q, want trimmed %q", msg.Content, "こんにちは")
	}
}

// 項番51-53: createdAt/updatedAt はサーバー時刻で設定され、呼び出し引数に
// クライアント指定の時刻を渡す経路が存在しない（Execute のシグネチャ上、時刻は
// 受け取らず内部で time.Now() を使う）ことを構造的に確認する。
func TestSendMessage_SetsCreatedAndUpdatedAtToServerTime(t *testing.T) {
	repo := newFakeMessageRepo()
	uc := newSendMessageUseCase(repo)

	before := time.Now()
	msg, err := uc.Execute(context.Background(), 1, 7, "こんにちは", nil)
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.CreatedAt.Before(before) || msg.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want between %v and %v", msg.CreatedAt, before, after)
	}
	if !msg.CreatedAt.Equal(msg.UpdatedAt) {
		t.Errorf("CreatedAt (%v) and UpdatedAt (%v) should match on creation", msg.CreatedAt, msg.UpdatedAt)
	}
}
