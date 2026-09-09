package model

import "time"

type Media struct {
	ID             int64
	UploaderUserID int64
	StorageKey     string
	ContentType    string
	// 画像の実寸。クライアントがアップロード時に申告し、既存分は
	// cmd/backfill-media-dimensions が埋める。動画や PDF など寸法を
	// 持たないメディア、および埋め戻し前の行では nil になる。
	Width     *int
	Height    *int
	CreatedAt time.Time
}
