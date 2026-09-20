package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// CourseRoomUnread pairs a course chat room with the caller's unread count in it.
type CourseRoomUnread struct {
	RoomID      int64
	UnreadCount int
}

// MessageCursor は一覧取得の起点。どれか1つだけを指定する（複数指定時の優先順位は
// 実装側のコメントを参照）。全部 nil なら最新ページ。
type MessageCursor struct {
	// BeforeID: このIDより古いメッセージを取る（過去方向へのページング）。
	BeforeID *int64
	// AfterID: このIDより新しいメッセージを取る（新着方向へのページング）。
	AfterID *int64
	// AfterTime: この時刻より後のメッセージを取る（未読位置からの読み直し）。
	AfterTime *time.Time
}

// MessageQuery はメッセージ一覧の取得条件。
// 裸の引数を並べると呼び出し側で beforeID/afterID の取り違えが起きるので、
// 意味のある1つの型にまとめている。
type MessageQuery struct {
	RoomID int64
	Limit  int
	Cursor MessageCursor
}

// MessagePage は一覧の1ページ。Items は常に古い順（id 昇順）。
// HasMoreBefore / HasMoreAfter は、このページの前後に未削除のメッセージが
// 実際に存在するかを表す（推定ではなく実測値）。
type MessagePage struct {
	Items         []*model.Message
	HasMoreBefore bool
	HasMoreAfter  bool
}

// MessageReader はメッセージ1件単位の読み取り。認可を前提としない
// （リゾルバや DataLoader から直接使う）ので、どの層へ渡しても構わない。
type MessageReader interface {
	// GetMessageByID returns the message, or nil if it does not exist or has been soft-deleted.
	GetMessageByID(ctx context.Context, id int64) (*model.Message, error)
	// GetMessagesByIDs returns the requested messages keyed by ID. IDs that do not
	// exist or were soft-deleted are simply absent from the map (引用返信の返信先取得用)。
	GetMessagesByIDs(ctx context.Context, ids []int64) (map[int64]*model.Message, error)
}

// MessageWriter はメッセージ1件単位の書き込み（保存・更新・論理削除）。
//
// 読み取り (MessageReader) と分けてあるのは配線の規律を型で表すため。この口を
// 渡してよい先は composition root (cmd/server/main.go) から
// usecase/chat.NewMessageWriters だけで、他のユースケースには MessageReader しか
// 渡さない。認可判定は usecase/chat が持っているので、そこを通らずに書き込める
// 依存を新しく作らないこと。
//
// 何を防げて何を防げないかは usecase/chat/writers.go のコメントに書いてある
// （型で塞げるのは「依存として受け取れるか」までで、同じパッケージ内や
// composition root からの直接呼び出しは塞げない）。
type MessageWriter interface {
	SaveMessage(ctx context.Context, m *model.Message) error
	UpdateMessage(ctx context.Context, m *model.Message) error
	// SoftDeleteMessage marks the message as deleted by deletedBy. It returns false
	// (with no error) if the message does not exist or was already deleted, so that
	// repeated delete attempts are idempotent and never corrupt deletedBy/deletedAt.
	SoftDeleteMessage(ctx context.Context, id int64, deletedBy int64) (bool, error)
}

// MessageReadModel は表示用の一覧取得。書き込みと違って
// 「どう並べてどこで切るか」だけが関心事なので分けている。
type MessageReadModel interface {
	// ListMessagesByRoomID は query の条件をそのまま SQL に落として1ページ返す。
	// 「どのカーソルを渡すと何が返るか」の仕様は usecase 側
	// （usecase/message/list_messages.go）に書いてある。
	ListMessagesByRoomID(ctx context.Context, query MessageQuery) (*MessagePage, error)
	GetLastMessagesByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error)
	// GetLatestMessageID はルームの最新メッセージIDを返す（1件も無ければ nil）。
	// 既読位置を指定しない markRoomAsRead（＝引数を知らない古いクライアント）の
	// フォールバックに使う。削除済みも含めた最大値を返す: 位置は「ここまでは見た」
	// というしおりで、ソフトデリートされた行を飛ばして小さい値を返すと、その行より
	// 後の既読が巻き戻る。
	GetLatestMessageID(ctx context.Context, roomID int64) (*int64, error)
	// MessageExistsInRoom は messageID が roomID のメッセージかを返す。
	// クライアントが指定してきた既読位置の検証に使う（他ルームのIDや存在しないIDで
	// 既読位置を壊されないため）。
	//
	// deleted_at で絞らないのは GetLatestMessageID と同じ理由。表示したあとに
	// 消されたメッセージのIDで既読を打つのは正常な流れで、そこで弾くと「既読に
	// できない部屋」ができてしまう。
	MessageExistsInRoom(ctx context.Context, roomID, messageID int64) (bool, error)
}

// MessageMentionStore はメッセージに紐づくメンション行の読み書き。
type MessageMentionStore interface {
	// CreateMessageMentions はメッセージに紐づくメンションを一括登録する（重複は無視）。
	CreateMessageMentions(ctx context.Context, messageID int64, mentions []*model.Mention) error
	DeleteMessageMentionsByMessageID(ctx context.Context, messageID int64) error
	// ListMentionsByMessageIDs はメッセージIDごとのメンション一覧を返す（DataLoader 用）。
	ListMentionsByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]*model.Mention, error)
}

// MessageUnreadCounter は未読バッジのための集計。既読位置そのものは
// room_users / course_room_reads 側が持ち、ここでは件数を数えるだけ。
//
// どの経路も未読の起点は UnreadOrigin の規則（ID 優先・時刻・フォールバック）に従う。
//
// ここにあるのは全て「呼び出し元1人ぶん」を数える経路であり、そう保つこと。
// 以前は送信のたびにルームの全メンバー・全履修者ぶんの未読数を数えて SSE で配る経路
// （CountUnreadMessagesPerMember / CountUnreadMessagesPerCourseRegistrant）があったが、
// 履修者数百人規模の授業では投稿1件ごとにその集計が走るうえ、受け取ったクライアントは
// どのみち自分ぶんを取り直していた。派生値を全員ぶん計算して配るのはやめ、SSE は
// 「このルームが更新された」という事実だけを配り、未読数は必要になった利用者が
// ここを通って自分ぶんだけ取る形にしてある。
type MessageUnreadCounter interface {
	// CountUnreadMessages は1ルームぶんを数える。起点は呼び出し側が既読位置から
	// 組み立てて渡す（既読位置の置き場が room_users / course_room_reads で分かれ、
	// 授業ルームだけフォールバックが時間割の登録時刻になるため）。
	CountUnreadMessages(ctx context.Context, roomID, userID int64, origin UnreadOrigin) (int, error)
	CountUnreadMessagesByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int, error)
	CountUnreadMessagesByRoomType(ctx context.Context, userID int64, roomType string) (int, error)
	// CountUnreadByCourseRooms returns the unread count for every course room in
	// userID's timetable for the given semester (room_id 昇順). 授業内チャットは
	// room_users を使わないため、既読位置は course_room_reads から取り、
	// まだ一度も開いていない授業は時間割に登録した時点を起点に数える。
	CountUnreadByCourseRooms(ctx context.Context, userID int64, year int, semester string) ([]*CourseRoomUnread, error)
}

// MessageRepository は上の口の合成。DI 配線（main.go）が1つの実装を渡せば済むように
// 残してあるだけで、各 usecase は自分が使う狭い口だけに依存すること。
// 特に、これを引数の型に取る層を新しく増やさないこと: 合成の口を受け取った時点で
// 書き込み (MessageWriter) も一緒に手に入ってしまい、読み書きを分けた意味が消える。
type MessageRepository interface {
	MessageReader
	MessageWriter
	MessageReadModel
	MessageMentionStore
	MessageUnreadCounter
}
