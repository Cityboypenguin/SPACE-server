package model

import "time"

type Administrator struct {
	ID             int64
	Name           string
	Email          string
	HashedPassword string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateAdministratorParam struct {
	Name      string
	Email     string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateAdministratorParam struct {
	Name     *string
	Email    *string
	Password *string
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
