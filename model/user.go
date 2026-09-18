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

// User は表示に使うユーザー。パスワードハッシュは持たない。
//
// 以前は HashedPassword もこの型にあり、表示用の取得（一覧・検索・ルームの
// メンバー・メンションの解決・DataLoader の UserLoader）まで軒並み
// hashed_password を SELECT していた。秘密が表示系のコードパスに載ると、
// 取り違えて外へ出す事故の可能性がその経路の数だけ増える。
//
// 秘密は UserCredentials にだけ置き、「この型を持っている限りハッシュを
// 触れない」ことを型で保証する。列を落とすだけにしなかったのは、構造体の
// フィールドが残っていると、SELECT を書き足した誰かが表示系にハッシュを
// 載せ直せてしまうため（それはコンパイルでは止まらない）。
//
// email は残してある。GraphQL の User.email が非 null の公開フィールドで、
// 表示系（toGraphUser）が必ず埋めているため、ここから外すと外から見える挙動が
// 変わる。今回の目的は「認証情報＝パスワードハッシュを表示系に載せない」こと。
type User struct {
	ID        int64
	AccountID string
	Name      string
	Email     string
	Role      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserCredentials は認証に使う秘密を伴うユーザー。
//
// これを取得してよいのは認証の経路だけ:
//   - ログイン（LoginUserUseCase）
//   - パスワード変更時の現在パスワード照合（UpdateUserUseCase）
//   - パスワード再設定（ResetPasswordUseCase）
//   - 新規登録（CreateUserUseCase）
//
// トークン検証（internal/auth.ValidateAndVerifyToken）は凍結状態しか見ないので
// 公開情報の GetUserByID を使う。認証まわりでも、秘密が要らないなら取らない。
type UserCredentials struct {
	User
	HashedPassword string
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

// CreateUser は新規登録。パスワードをハッシュ化するので UserCredentials 側にある。
func (c *UserCredentials) CreateUser(param CreateUserParam) error {
	if err := ValidateAccountID(param.AccountID); err != nil {
		return err
	}
	if err := ValidateUserName(param.Name); err != nil {
		return err
	}

	hashedPassword, err := hashPassword(param.Password)
	if err != nil {
		return err
	}

	c.AccountID = param.AccountID
	c.Name = param.Name
	c.Email = param.Email
	c.HashedPassword = hashedPassword
	c.CreatedAt = param.CreatedAt
	c.UpdatedAt = param.UpdatedAt

	return nil
}

// UpdateProfile は公開情報だけを更新する（表示系の更新経路用）。
//
// パスワードを渡されたら黙って無視せずエラーにする。無視すると「変更したのに
// 変わっていない」という気づけない壊れ方になるので、秘密を扱える
// UserCredentials.UpdateUser を使えと明示的に知らせる。
func (u *User) UpdateProfile(param UpdateUserParam) error {
	if param.Password != nil {
		return fmt.Errorf("password changes must go through UserCredentials.UpdateUser")
	}
	return u.applyPublicUpdate(param)
}

func (u *User) applyPublicUpdate(param UpdateUserParam) error {
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
	u.UpdatedAt = time.Now()
	return nil
}

// UpdateUser は公開情報とパスワードをまとめて更新する（認証の経路用）。
func (c *UserCredentials) UpdateUser(param UpdateUserParam) error {
	if err := c.applyPublicUpdate(param); err != nil {
		return err
	}
	if param.Password != nil {
		hashedPassword, err := hashPassword(*param.Password)
		if err != nil {
			return err
		}
		c.HashedPassword = hashedPassword
	}
	return nil
}
