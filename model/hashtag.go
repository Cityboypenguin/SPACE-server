package model

// HashtagSuggestion はハッシュタグのサジェスト1件（タグ名と使用回数）を表す。
type HashtagSuggestion struct {
	Tag   string
	Count int
}
