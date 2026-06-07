package model

import "time"

type Profile struct {
	UserID      int64
	Bio         string
	AvatarMedia *Media
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpdateProfileParam struct {
	Bio *string
}

func (p *Profile) UpdateProfile(param UpdateProfileParam) {
	if param.Bio != nil {
		p.Bio = *param.Bio
	}
	p.UpdatedAt = time.Now()
}
