package model

import (
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

// Message はチャット1件。
//
// ここに持たせるのは「メッセージ自身の情報だけで判断できること」に限る。
// 「授業の学期が終わっていないか」「履修しているか」「匿名表示にするか」といった
// room / course の知識が要る判定は、メッセージ単体では決められないので
// usecase 側（CheckRoomWritableUseCase など）に置く。
type Message struct {
	ID     int64
	RoomID int64
	UserID int64
	// AuthorRole は投稿時点での投稿者の立場（STUDENT / TEACHER）。
	// 質問・回答・投票と同じく、あとから利用者の属性が変わっても
	// 「そのとき誰として書いたか」が変わらないよう行に焼き付ける。
	AuthorRole string
	// ReplyToID は引用返信の返信先メッセージID。返信でない場合は nil。
	// 返信先がソフトデリートされても値は残るため、表示側は返信先を取得できない
	// ケース（削除済み）を扱う必要がある。
	ReplyToID *int64
	Content   string
	// Mentions は本文中のメンション。書き込み時に解決したものを保持し、
	// 読み出し時は必要なときだけ（GraphQL の mentions フィールド解決時に）埋める。
	Mentions  []*Mention
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	DeletedBy *int64
}

// IsDeleted reports whether the message has been soft-deleted.
func (m *Message) IsDeleted() bool {
	return m.DeletedAt != nil
}

// IsInRoom は、このメッセージが roomID のルームのものかを返す。
// 「他ルームのメッセージIDを指定して内容を覗く」類の取り違えを防ぐ照合を
// 呼び出し側で `msg.RoomID != roomID` と書き散らかさないための口。
func (m *Message) IsInRoom(roomID int64) bool {
	return m.RoomID == roomID
}

// CanBeEditedBy は userID がこのメッセージを編集してよいかを返す。
// 編集できるのは本人（と管理者）だけで、削除済みのメッセージは誰も編集できない。
//
// ルーム種別に依存する制限（授業内チャットは学期が終わったら編集不可、など）は
// メッセージ単体では判断できないので、ここでは扱わず呼び出し側で重ねて確認する。
func (m *Message) CanBeEditedBy(userID int64, isAdmin bool) bool {
	if m.IsDeleted() {
		return false
	}
	return m.UserID == userID || isAdmin
}

// CanBeDeletedBy は userID がこのメッセージを削除してよいかを返す。
// 判断基準は編集と同じ（本人か管理者、削除済みは不可）。
// コミュニティのオーナーによる他人のメッセージ削除は room のメンバー役割を
// 見ないと決まらないため、ここでは扱わず呼び出し側で重ねて確認する。
func (m *Message) CanBeDeletedBy(userID int64, isAdmin bool) bool {
	return m.CanBeEditedBy(userID, isAdmin)
}

type CreateMessageParam struct {
	RoomID    int64
	UserID    int64
	ReplyToID *int64
	Content   string
	// MediaKeys は添付のストレージキー。本文が空でも添付があれば送信できるため、
	// 「本文と添付が両方空」を弾くためにここで受け取る（保存自体は media 側の責務）。
	MediaKeys []string
	// AuthorRole は省略可。空なら AuthorRoleStudent を既定にする
	// （現状の投稿者は全員学生で、教員アカウントは未導入のため）。
	AuthorRole string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type UpdateMessageParam struct {
	Content *string
}

// NewMessage は保存前のメッセージを組み立てる。
// 「本文も添付も無いメッセージは存在しない」という不変条件をここで閉じ込め、
// 生成経路（通常送信・将来の別経路）が増えても検証が漏れないようにする。
func NewMessage(param CreateMessageParam) (*Message, error) {
	content := strings.TrimSpace(param.Content)
	if content == "" && len(param.MediaKeys) == 0 {
		return nil, apperr.InvalidInput("content or media is required")
	}

	authorRole := param.AuthorRole
	if authorRole == "" {
		authorRole = AuthorRoleStudent
	}

	createdAt := param.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	updatedAt := param.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	return &Message{
		RoomID:     param.RoomID,
		UserID:     param.UserID,
		AuthorRole: authorRole,
		ReplyToID:  param.ReplyToID,
		Content:    content,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

func (m *Message) UpdateMessage(param UpdateMessageParam) {
	if param.Content != nil {
		m.Content = *param.Content
	}
	m.UpdatedAt = time.Now()
}
