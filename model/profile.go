package model

import "time"

type Profile struct {
	UserID    int64
	Bio       string
	Image     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateProfileParam struct {
	Bio   *string
	Image *string
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
