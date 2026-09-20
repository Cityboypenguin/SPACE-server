package model

// Mention は投稿/メッセージ本文中の1件のメンションを表す。
//
// UserID は書き込み時に解決したメンション先のユーザーID、Text は本文に書かれた
// 表記のスナップショット（投稿は accountID、コミュニティは表示名）。
// 相手が accountID や表示名を変更しても本文の文字列は変わらないため、
// 表示側の着色・リンク化は Text との突き合わせで行い、遷移先は UserID を使う。
// こうすることで「改名でリンクが切れる」「ID再取得した別人に飛ぶ」の両方を防げる。
type Mention struct {
	UserID int64
	Text   string
}
