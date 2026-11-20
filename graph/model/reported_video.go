package model

import domain "github.com/Cityboypenguin/SPACE-server/model"

// ReportedVideo は通報の写しの動画（GraphQL の ReportedVideo）。
// available と thumbnailURL はリゾルバが Source から求める。
type ReportedVideo struct {
	ID         string  `json:"ID"`
	DurationMs int32   `json:"durationMs"`
	Width      int32   `json:"width"`
	Height     int32   `json:"height"`
	HasAudio   bool    `json:"hasAudio"`
	SizeBytes  float64 `json:"sizeBytes"`
	CreatedAt  string  `json:"createdAt"`

	Source *domain.ReportedContentVideo `json:"-"`
}
