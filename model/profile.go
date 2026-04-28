package model

import "time"

// Profile はデータベースに保存するプロフィールの「箱」です
type Profile struct {
	UserID    int64  `json:"user_id"`
	Bio       string `json:"bio"`
	Grade     int32  `json:"grade"`
	Image     string `json:"image"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// UpdateProfileParam は、受付から現場監督へ更新データを渡すときの「箱」です
// 中身が空っぽ（nil）の可能性があるため、すべてポインタ（*）にしています
type UpdateProfileParam struct {
	Bio   *string `json:"bio"`
	Grade *int32  `json:"grade"`
	Image *string `json:"image"`
}

// UpdateProfile は、古いプロフィールに新しいデータを上書きする処理です
func (p *Profile) UpdateProfile(param UpdateProfileParam) {
	if param.Bio != nil {
		p.Bio = *param.Bio
	}
	if param.Grade != nil {
		p.Grade = *param.Grade
	}
	if param.Image != nil {
		p.Image = *param.Image
	}
	p.UpdatedAt = time.Now().Unix()
}
