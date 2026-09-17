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

// MessageStore はメッセージ1件単位の読み書き（書き込み系の正）。
type MessageStore interface {
	SaveMessage(ctx context.Context, m *model.Message) error
	// GetMessageByID returns the message, or nil if it does not exist or has been soft-deleted.
	GetMessageByID(ctx context.Context, id int64) (*model.Message, error)
	// GetMessagesByIDs returns the requested messages keyed by ID. IDs that do not
	// exist or were soft-deleted are simply absent from the map (引用返信の返信先取得用)。
	GetMessagesByIDs(ctx context.Context, ids []int64) (map[int64]*model.Message, error)
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
type MessageUnreadCounter interface {
	CountUnreadMessages(ctx context.Context, roomID, userID int64, afterTimestamp int64) (int, error)
	CountUnreadMessagesByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int, error)
	CountUnreadMessagesByRoomType(ctx context.Context, userID int64, roomType string) (int, error)
	CountUnreadMessagesPerMember(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
	// CountUnreadByCourseRooms returns the unread count for every course room in
	// userID's timetable for the given semester (room_id 昇順). 授業内チャットは
	// room_users を使わないため、既読位置は course_room_reads から取り、
	// まだ一度も開いていない授業は時間割に登録した時点を起点に数える。
	CountUnreadByCourseRooms(ctx context.Context, userID int64, year int, semester string) ([]*CourseRoomUnread, error)
}

// MessageRepository は上の4つの合成。DI 配線（main.go）が1つの実装を渡せば
// 済むように残してあるだけで、各 usecase は自分が使う狭い口だけに依存すること。
type MessageRepository interface {
	MessageStore
	MessageReadModel
	MessageMentionStore
	MessageUnreadCounter
}
