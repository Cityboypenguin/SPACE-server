package model

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	MaxUserNameLength  = 50
	MaxAccountIDLength = 25
)

const (
	UserStatusActive = "active"
	UserStatusFrozen = "frozen"
)

type User struct {
	ID             int64
	AccountID      string
	Name           string
	Email          string
	HashedPassword string
	Role           string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateUserParam struct {
	AccountID string
	Name      string
	Email     string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateUserParam struct {
	AccountID *string
	Name      *string
	Email     *string
	Password  *string
}

// 有効な学科記号（表6より。LG=2020募集停止、LZ=2019募集停止のため除外）
// 2文字コードを E より先に記述して誤マッチを防ぐ
var studentEmailRe = regexp.MustCompile(`(?i)^(EE|EL|EW|JL|JP|MA|MD|CM|CA|LB|LA|LT|LR|LK|LM|NE|HP|HS|GN|GC|E)(2[0-9]|[3-9][0-9])\d{4}@senshu-u\.jp$`)

func ValidateUserEmail(email string) error {
	if !studentEmailRe.MatchString(email) {
		return fmt.Errorf("メールアドレスは2020年度以降の学籍番号形式のみ登録できます")
	}
	return nil
}

func ValidateUserPassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("パスワードは8文字以上で入力してください")
	}
	return nil
}

var accountIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ValidateAccountID(accountID string) error {
	if !accountIDRe.MatchString(accountID) {
		return fmt.Errorf("ユーザーIDは半角英数字・_・-のみ使用できます")
	}
	if utf8.RuneCountInString(accountID) > MaxAccountIDLength {
		return fmt.Errorf("ユーザーIDは%d文字以内で入力してください", MaxAccountIDLength)
	}
	return nil
}

func ValidateUserName(name string) error {
	if utf8.RuneCountInString(name) > MaxUserNameLength {
		return fmt.Errorf("名前は%d文字以内で入力してください", MaxUserNameLength)
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

func (u *User) CreateUser(param CreateUserParam) error {
	hashedPassword, err := hashPassword(param.Password)
	if err != nil {
		return err
	}

	u.AccountID = param.AccountID
	u.Name = param.Name
	u.Email = param.Email
	u.HashedPassword = hashedPassword
	u.CreatedAt = param.CreatedAt
	u.UpdatedAt = param.UpdatedAt

	return nil
}

func (u *User) UpdateUser(param UpdateUserParam) error {
	if param.AccountID != nil {
		if err := ValidateAccountID(*param.AccountID); err != nil {
			return err
		}
		u.AccountID = *param.AccountID
	}
	if param.Name != nil {
		if err := ValidateUserName(*param.Name); err != nil {
			return err
		}
		u.Name = *param.Name
	}
	if param.Email != nil {
		u.Email = *param.Email
	}
	if param.Password != nil {
		hashedPassword, err := hashPassword(*param.Password)
		if err != nil {
			return err
		}
		u.HashedPassword = hashedPassword
	}
	u.UpdatedAt = time.Now()
	return nil
}
