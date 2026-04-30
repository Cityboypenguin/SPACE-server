package model

import "time"

type Administrator struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"hashed_password"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateAdministratorParam struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateAdministratorParam struct {
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
}

func (a *Administrator) CreateAdministrator(param CreateAdministratorParam) error {
	hashedPassword, err := hashPassword(param.Password)
	if err != nil {
		return err
	}

	a.Name = param.Name
	a.Email = param.Email
	a.HashedPassword = hashedPassword
	a.CreatedAt = param.CreatedAt
	a.UpdatedAt = param.UpdatedAt

	return nil
}

func (a *Administrator) UpdateAdministrator(param UpdateAdministratorParam) error {
	if param.Name != nil {
		a.Name = *param.Name
	}
	if param.Email != nil {
		a.Email = *param.Email
	}
	if param.Password != nil {
		hashedPassword, err := hashPassword(*param.Password)
		if err != nil {
			return err
		}
		a.HashedPassword = hashedPassword
	}

	return nil
}
