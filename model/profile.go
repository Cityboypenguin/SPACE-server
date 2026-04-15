package model

import "time"

// Profile はデータベースに保存するプロフィールの「箱」です
type Profile struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Bio       string `json:"bio"`
	Grade     string `json:"grade"`
	Image     string `json:"image"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// UpdateProfileParam は、受付から現場監督へ更新データを渡すときの「箱」です
// 中身が空っぽ（nil）の可能性があるため、すべてポインタ（*）にしています
type UpdateProfileParam struct {
	Username *string `json:"username"`
	Bio      *string `json:"bio"`
	Grade    *string `json:"grade"`
	Image    *string `json:"image"`
}

// UpdateProfile は、古いプロフィールに新しいデータを上書きする処理です
func (p *Profile) UpdateProfile(param UpdateProfileParam) {
	if param.Username != nil {
		p.Username = *param.Username
	}
	if param.Bio != nil {
		p.Bio = *param.Bio
	}
	if param.Grade != nil {
		p.Grade = *param.Grade
	}
	if param.Image != nil {
		p.Image = *param.Image
	}
	// 更新日時を「今」に書き換える
	p.UpdatedAt = time.Now().Unix()
}
