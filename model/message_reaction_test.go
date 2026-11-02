package model

import "testing"

func TestIsAllowedMessageReactionEmoji(t *testing.T) {
	if IsAllowedMessageReactionEmoji("👍") {
		t.Error("plain emojis are not on the list any more (スタンプだけにした)")
	}
	if !IsAllowedMessageReactionEmoji("stamp:ryokai") {
		t.Error("stamp:ryokai (文字スタンプ) must be allowed")
	}
	for _, emoji := range []string{"", " ", "👍 ", "🖕", "👎", "abc", "stamp:", "stamp:unknown", "Stamp:ok"} {
		if IsAllowedMessageReactionEmoji(emoji) {
			t.Errorf("%q must not be allowed", emoji)
		}
	}
}

func TestAllowedMessageReactionEmojis_ReturnsACopy(t *testing.T) {
	first := AllowedMessageReactionEmojis()
	if len(first) == 0 {
		t.Fatal("the allowed list must not be empty")
	}
	first[0] = "🖕"
	if IsAllowedMessageReactionEmoji("🖕") || !IsAllowedMessageReactionEmoji("stamp:thanks") {
		t.Error("changing the returned slice must not change the allowed list")
	}
}
