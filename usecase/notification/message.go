package notification

import "unicode/utf8"

// 通知の文言はこのファイルに集める。
//
// 以前は graph（チャットの返信・メンション、コミュニティの権限変更、フォロー）と
// usecase（投稿への返信・投稿でのメンション・いいね）の2層6ファイルに散っていた。
// 散っていると、(a) 「投稿では『投稿で』と言うのにチャットでは何と言うか」を揃えるのに
// 全ファイルを grep するしかなく、(b) 通知を1つ足すたびにその場の流儀で文言が書かれ、
// (c) 言い回しを直す作業が層をまたぐ。文言は「利用者に何を知らせるか」という通知
// そのものの持ち物なので、配信の手段（graph / SSE）ではなく通知の usecase に置く。
//
// 保存される値であることに注意。notifications.message は組み立て済みの文字列を
// そのまま持つので、ここを直しても既に配った通知の文面は変わらない（過去の通知は
// 当時の言い回しのまま残る）。
//
// 通知タイプ（TypeMention など）との対応は publisher.go を参照。
//
// ここに無いもの: DM とお知らせの通知文。どちらも固定の言い回しではなく中身そのもの
// （DM はメッセージ本文のプレビュー、お知らせはその題名）を文面に使うので、文言の
// 置き場ではなくそれを持っている側で組む。

// 場所を名乗らない通知。遷移先（投稿・コミュニティ・プロフィール）を開けば
// どこの話か分かるので、文言では出来事だけを伝える。
const (
	MessageMentionedInPost = "投稿であなたがメンションされました"
	MessageRepliedToPost   = "あなたの投稿に返信がありました"
	MessageFavoritedPost   = "あなたの投稿がいいねされました"
	MessageFollowed        = "あなたがお気に入りされました"

	MessagePromotedToCommunityOwner  = "コミュニティのオーナーに昇格しました"
	MessageDemotedFromCommunityOwner = "コミュニティのオーナーから降格されました"
	MessageKickedFromCommunity       = "コミュニティからキックされました"
)

// MessageMentionedInRoom はチャットでメンションされたときの文言。
func MessageMentionedInRoom(roomName string) string {
	// 場所を名乗れないときは種別だけを言う。メンションは投稿でも起きるので、
	// 何も言わないと「どこでメンションされたか」の手がかりが通知から消える。
	return inRoom(roomName, "コミュニティ", "あなたがメンションされました")
}

// MessageRepliedInRoom はチャットで引用返信されたときの文言。
func MessageRepliedInRoom(roomName string) string {
	// 「メッセージに返信」と言っている時点でチャットの話だと分かるので、
	// 場所を名乗れないときは場所に触れない。
	return inRoom(roomName, "", "あなたのメッセージに返信がありました")
}

// MessageAnonymousRepliedInRoom は授業内チャット（匿名）で引用返信されたときの文言。
//
// anonymousLabel はそのルーム内の匿名ラベル（「匿名12」など）。ここに実名を渡しては
// いけない。授業内チャットの匿名性は通知の文言からも崩れうる（誰が返信したかが
// 分かってしまう）ので、呼び出し側は必ずラベルを解決してから渡すこと。
func MessageAnonymousRepliedInRoom(roomName, anonymousLabel string) string {
	return inRoom(roomName, "", anonymousLabel+"さんがあなたのメッセージに返信しました")
}

// 通知に載せるルーム名の上限。ルーム名は255文字まで入りうる一方、通知は一覧でも
// トーストでも1行で出るので、そのまま載せると肝心の「何が起きたか」が画面から
// 押し出される。
const maxRoomNameRunes = 20

// inRoom は「<場所>で<出来事>」という文言を組む。event は「〜で」に続く節。
//
// 場所には rooms.name をそのまま使う。コミュニティ名・授業名そのもの（ルームはその
// 名前で作られ、コミュニティを改名すればルーム名も一緒に更新される）なので、出せば
// 受け取った側が場所を特定できる。
//
// fallbackPlace は名前が空のときに代わりに名乗る場所（種別の呼び名）。空文字を渡すと
// 場所を言わず event だけを返す。「」で という空の場所を出すよりはまだ読めるため。
// この道は実際に通りうる: rooms.name は NOT NULL だが空文字は入れられ、コミュニティ名を
// 非空にする検証がサーバ側に無いので、名前なしのコミュニティを作れば空のまま届く。
//
// 「で」で繋ぐという文法を知っている場所をここ1つに留めているのは、呼び出し側が
// それぞれ連結と空判定を書くと、通知の種類が増えるたびに同じ判断が写され、
// 片方だけ直し忘れるため。
func inRoom(roomName, fallbackPlace, event string) string {
	place := truncateRunes(roomName, maxRoomNameRunes)
	if place == "" {
		if fallbackPlace == "" {
			return event
		}
		return fallbackPlace + "で" + event
	}
	return "「" + place + "」で" + event
}

// truncateRunes は maxRunes を超える文字列を切り詰めて末尾に … を付ける。
// バイト数ではなく文字数で測る（日本語は1文字3バイトなので、バイトで切ると
// 文字の途中で切れて壊れた字が出る）。
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes]) + "…"
}
