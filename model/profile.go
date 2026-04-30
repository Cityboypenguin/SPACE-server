package model

import "time"

type Profile struct {
	UserID    int64     `json:"user_id"`
	Bio       string    `json:"bio"`
	Image     string    `json:"image"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateProfileParam struct {
	Bio   *string `json:"bio"`
	Image *string `json:"image"`
}

func (p *Profile) UpdateProfile(param UpdateProfileParam) {
	if param.Bio != nil {
		p.Bio = *param.Bio
	}
	if param.Image != nil {
		p.Image = *param.Image
	}
	p.UpdatedAt = time.Now()
}
