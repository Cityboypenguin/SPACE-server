package notification

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 場所を名乗れる通知は、ルーム名を文言に入れる。
// ここが抜けると、受け取った側は通知を開くまでどのルームの話か分からない。
func TestMessagesNameTheRoom(t *testing.T) {
	const room = "プログラミング部"

	for _, tt := range []struct {
		name string
		got  string
	}{
		{"mention", MessageMentionedInRoom(room)},
		{"reply", MessageRepliedInRoom(room)},
		{"anonymous reply", MessageAnonymousRepliedInRoom(room, "匿名12")},
	} {
		if !strings.Contains(tt.got, room) {
			t.Errorf("%s: message = %q, want it to name the room %q", tt.name, tt.got, room)
		}
	}
}

// 匿名ルームの返信は、場所を名乗っても匿名ラベルのままであること。
// 場所を言うために実名を漏らしては元も子もない（実名を渡さないのは呼び出し側の責任だが、
// ラベルがそのまま文面に出ることはここで固定しておく）。
func TestMessageAnonymousRepliedInRoom_KeepsTheLabel(t *testing.T) {
	got := MessageAnonymousRepliedInRoom("情報工学概論", "匿名12")
	if !strings.Contains(got, "匿名12さん") {
		t.Errorf("message = %q, want it to use the anonymous label", got)
	}
}

// ルーム名が空のときの倒し方は通知ごとに違う。メンションは投稿でも起きるので種別
// （コミュニティ）だけでも名乗り、返信は「メッセージに返信」で文脈が分かるので場所に
// 触れない。どちらも「」で という空の場所を出さないこと。
//
// この道は実際に通りうる: コミュニティ名を非空にする検証がサーバ側に無いため、
// 名前なしのコミュニティを作れば rooms.name は空文字のまま届く。
func TestMessagesFallBackWithoutARoomName(t *testing.T) {
	if got := MessageMentionedInRoom(""); got != "コミュニティであなたがメンションされました" {
		t.Errorf("mention = %q, want the fallback place", got)
	}
	if got := MessageRepliedInRoom(""); got != "あなたのメッセージに返信がありました" {
		t.Errorf("reply = %q, want just the event", got)
	}
	if got := MessageAnonymousRepliedInRoom("", "匿名12"); got != "匿名12さんがあなたのメッセージに返信しました" {
		t.Errorf("anonymous reply = %q, want just the event", got)
	}
}

// 長いルーム名は切り詰める。通知は1行で出るので、名前に押し出されて
// 「何が起きたか」が読めなくなってはいけない。
func TestMessagesTruncateALongRoomName(t *testing.T) {
	name := strings.Repeat("あ", 40)
	got := MessageMentionedInRoom(name)

	if !strings.Contains(got, "…」で") {
		t.Errorf("message = %q, want a truncated name ending with …", got)
	}
	if utf8.RuneCountInString(got) >= utf8.RuneCountInString(name) {
		t.Errorf("message = %q (%d runes), want it shorter than the raw name", got, utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "あなたがメンションされました") {
		t.Errorf("message = %q, want the event clause kept intact", got)
	}
}

// 切り詰めは文字数で測る（日本語は1文字3バイトなので、バイトで切ると字が壊れる）。
func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("あいう", 3); got != "あいう" {
		t.Errorf("truncateRunes = %q, want the string untouched at the limit", got)
	}
	if got := truncateRunes("あいう", 2); got != "あい…" {
		t.Errorf("truncateRunes = %q, want it cut on a rune boundary", got)
	}
	if !utf8.ValidString(truncateRunes(strings.Repeat("あ", 30), 20)) {
		t.Error("truncateRunes produced invalid UTF-8; バイト数で切ると字が壊れる")
	}
}
