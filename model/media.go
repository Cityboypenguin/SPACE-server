package model

import "time"

type Media struct {
	ID             int64
	UploaderUserID int64
	StorageKey     string
	ContentType    string
	// 画像の実寸。アップロード時にクライアントが申告し、それ以前のメディアは
	// 表示できたブラウザからの報告（reportMediaDimensions）で埋まる。
	// 動画や PDF など寸法を持たないメディア、および未報告の行では nil になる。
	Width     *int
	Height    *int
	CreatedAt time.Time
}

// 実在しうる画像の一辺の上限。これを超える申告は誤りか悪意とみなして捨てる。
// 実用上の最大級（数億画素クラス）でも 1 辺 65535px を超えることはまずない。
const maxImageDimension = 65535

// ValidImageDimension は申告された一辺の長さが妥当かを返す。
// 寸法はクライアントの観測値であり、表示領域のヒントにしか使わないため、
// 範囲外は誤りとして捨て「寸法未取得」と同じ扱いに落とす。
func ValidImageDimension(v int) bool {
	return v > 0 && v <= maxImageDimension
}

// MediaInput はアップロード済みオブジェクトへの参照。投稿・メッセージ・質問・回答の
// いずれの添付でも形は同じなので、ユースケースごとに定義せずここに集約する。
// Media を新しい属性で拡張するときに触る箇所を1つに保つのが目的。
type MediaInput struct {
	StorageKey  string
	ContentType string
	// 画像の実寸。クライアントの申告値で、表示側のレイアウト確保にのみ使う。
	// 申告が無い場合は nil。
	Width  *int
	Height *int
}

// NewMedia は入力から保存用の Media を組み立てる。
func NewMedia(uploaderUserID int64, input MediaInput, createdAt time.Time) *Media {
	return &Media{
		UploaderUserID: uploaderUserID,
		StorageKey:     input.StorageKey,
		ContentType:    input.ContentType,
		Width:          input.Width,
		Height:         input.Height,
		CreatedAt:      createdAt,
	}
}
