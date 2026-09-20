package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	_ "github.com/go-sql-driver/mysql"
)

// 表示系の SELECT に hashed_password も email も紛れ込んでいないことを、
// 列リストの定数そのもので固定する。
//
// 型（model.User がどちらも持てない）でも守っているが、型だけでは「読むだけ
// 読んで捨てる」SELECT を止められない。秘密と個人情報を DB から引いてくること
// 自体を止めたいので、列リストを定数にしてここで見張る。
func TestUserColumnProjectionsSeparateCredentials(t *testing.T) {
	if strings.Contains(userPublicColumns, "hashed_password") {
		t.Errorf("userPublicColumns に hashed_password が入っている: %s", userPublicColumns)
	}
	if !strings.Contains(userCredentialColumns, "hashed_password") {
		t.Errorf("userCredentialColumns に hashed_password が無い: %s", userCredentialColumns)
	}
	if strings.Contains(roomMemberUserColumns, "hashed_password") {
		t.Errorf("roomMemberUserColumns に hashed_password が入っている: %s", roomMemberUserColumns)
	}

	// 公開情報の列の並びは scanUser / scanUsers / room_users の JOIN が共有して
	// いるので、数がずれたら実行時の列数不一致になる。
	if got, want := len(strings.Split(userPublicColumns, ",")), 7; got != want {
		t.Errorf("userPublicColumns の列数 = %d, want %d", got, want)
	}
	if got, want := len(strings.Split(roomMemberUserColumns, ",")), 7; got != want {
		t.Errorf("roomMemberUserColumns の列数 = %d, want %d", got, want)
	}
}

// (A) の要。表示系の SELECT が他人のメールアドレスを引かないことを列リストで固定する。
//
// 以前は userPublicColumns に email があり、投稿の作者・ルームのメンバー・検索結果を
// 引くたびに他人の連絡先が DB から持ち上がっていた（そして GraphQL の User.email が
// 非 null の公開フィールドだったので、実際に外へ出てもいた）。
// hashed_password と同じ扱いにして、要らない経路ではそもそも引かない。
func TestUserColumnProjectionsSeparateEmail(t *testing.T) {
	if strings.Contains(userPublicColumns, "email") {
		t.Errorf("userPublicColumns に email が入っている（表示系が他人の連絡先を引いてしまう）: %s", userPublicColumns)
	}
	if strings.Contains(roomMemberUserColumns, "email") {
		t.Errorf("roomMemberUserColumns に email が入っている（メンバー一覧が他人の連絡先を引いてしまう）: %s", roomMemberUserColumns)
	}
	if !strings.Contains(userAccountColumns, "email") {
		t.Errorf("userAccountColumns に email が無い（本人・管理者向けの取得が成り立たない）: %s", userAccountColumns)
	}
	if !strings.Contains(userCredentialColumns, "email") {
		t.Errorf("userCredentialColumns に email が無い（ログインの照合が成り立たない）: %s", userCredentialColumns)
	}

	// 3段の列リストは上に1列ずつ足しただけの関係。scanUserAccount /
	// scanUserCredentials は公開列の並びをそのまま流用して末尾を読み足すので、
	// この関係が崩れると Scan の順番がずれる（コンパイルでは止まらない）。
	if got, want := len(strings.Split(userAccountColumns, ",")), 8; got != want {
		t.Errorf("userAccountColumns の列数 = %d, want %d", got, want)
	}
	if got, want := len(strings.Split(userCredentialColumns, ",")), 10; got != want {
		t.Errorf("userCredentialColumns の列数 = %d, want %d", got, want)
	}
	if !strings.HasPrefix(userAccountColumns, userPublicColumns) {
		t.Errorf("userAccountColumns が userPublicColumns で始まっていない: %s", userAccountColumns)
	}
	if !strings.HasPrefix(userCredentialColumns, userAccountColumns) {
		t.Errorf("userCredentialColumns が userAccountColumns で始まっていない: %s", userCredentialColumns)
	}
}

// --- MySQL を使う統合テスト（DSN が渡されたときだけ走る）--------------------
//
// 列リストの定数を分けただけでは、SELECT する列と Scan する変数の数がずれた
// ときに気づけない（Go のコンパイルは通り、実行してはじめて落ちる）。
// 表示系の取得を実際に MySQL へ投げて、全経路が動くことと、認証情報側だけが
// ハッシュを返すことを確かめる。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run UserProjection
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。開発用の
// space スキーマには一切触らない。
func userProjectionTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed user projection test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("space_userproj_test_%d", os.Getpid())
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to drop the throwaway schema: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to create the throwaway schema: %v", err)
	}

	db, err := sql.Open("mysql", dsn+schema)
	if err != nil {
		root.Close()
		t.Fatalf("failed to open the throwaway schema: %v", err)
	}

	// db/migrations の DDL から外部キーだけ落とした形。
	ddl := []string{
		`CREATE TABLE users (
			id BIGINT NOT NULL AUTO_INCREMENT,
			account_id VARCHAR(255) NOT NULL UNIQUE,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL UNIQUE,
			hashed_password VARCHAR(255) NOT NULL,
			credentials_version BIGINT NOT NULL DEFAULT 0,
			role VARCHAR(50) NOT NULL DEFAULT 'student',
			status VARCHAR(50) NOT NULL DEFAULT 'active',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			last_active_at BIGINT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE rooms (
			id BIGINT NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE room_users (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			role VARCHAR(50) NOT NULL DEFAULT 'member',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			last_read_at BIGINT NULL,
			last_read_message_id BIGINT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_room_user (room_id, user_id)
		)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			root.Exec("DROP DATABASE " + schema)
			root.Close()
			t.Fatalf("failed to create a table: %v\n%s", err, stmt)
		}
	}

	return db, func() {
		db.Close()
		root.Exec("DROP DATABASE " + schema)
		root.Close()
	}
}

func TestUserProjection_DisplayPathsWorkWithoutCredentialColumns(t *testing.T) {
	db, cleanup := userProjectionTestDB(t)
	defer cleanup()

	ctx := context.Background()
	userRepo := NewMySQLUserRepository(db)
	roomUserRepo := NewMySQLRoomUserRepository(db)

	const hash = "$2a$10$abcdefghijklmnopqrstuv"
	creds := &model.UserCredentials{
		UserAccount: model.UserAccount{
			User: model.User{
				AccountID: "taro",
				Name:      "太郎",
				Role:      "user",
				Status:    model.UserStatusActive,
			},
			Email: "taro@example.com",
		},
		HashedPassword: hash,
	}
	if err := userRepo.SaveCredentials(ctx, creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	if creds.ID == 0 {
		t.Fatal("SaveCredentials did not assign an ID")
	}

	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO rooms (id, name, type, created_at, updated_at) VALUES (1, 'r', 'community', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (1, ?, 'member', ?, ?)`, creds.ID, now, now); err != nil {
		t.Fatalf("insert room_user: %v", err)
	}

	// 表示系はどれも動く（列と Scan の数がずれていない）。
	t.Run("公開情報の取得が全経路で動く", func(t *testing.T) {
		if u, err := userRepo.GetUserByID(ctx, creds.ID); err != nil || u == nil || u.AccountID != "taro" {
			t.Fatalf("GetUserByID = %+v, %v", u, err)
		}
		if us, err := userRepo.GetUsersByIDs(ctx, []int64{creds.ID}); err != nil || len(us) != 1 {
			t.Fatalf("GetUsersByIDs = %d, %v", len(us), err)
		}
		if u, err := userRepo.FindByEmail(ctx, "taro@example.com"); err != nil || u == nil {
			t.Fatalf("FindByEmail = %+v, %v", u, err)
		}
		if us, total, err := userRepo.ListUserAccounts(ctx, repository.PageQuery{Limit: 10, WithTotal: true}); err != nil || len(us) != 1 || total != 1 {
			t.Fatalf("ListUserAccounts = %d/%d, %v", len(us), total, err)
		}
		if us, _, err := userRepo.SearchUsersByKeyword(ctx, "太郎", repository.PageQuery{Limit: 10}); err != nil || len(us) != 1 {
			t.Fatalf("SearchUsersByKeyword = %d, %v", len(us), err)
		}
		if us, err := userRepo.GetUsersByAccountIDs(ctx, []string{"taro"}); err != nil || len(us) != 1 {
			t.Fatalf("GetUsersByAccountIDs = %d, %v", len(us), err)
		}
		if us, err := userRepo.SuggestUsersByPrefix(ctx, "ta", 10); err != nil || len(us) != 1 {
			t.Fatalf("SuggestUsersByPrefix = %d, %v", len(us), err)
		}
		if m, err := roomUserRepo.ListUsersByRoomIDs(ctx, []int64{1}); err != nil || len(m[1]) != 1 {
			t.Fatalf("ListUsersByRoomIDs = %v, %v", m, err)
		}
		if members, err := roomUserRepo.ListRoomMembersWithRoles(ctx, 1); err != nil || len(members) != 1 {
			t.Fatalf("ListRoomMembersWithRoles = %d, %v", len(members), err)
		}
	})

	// 本人・管理者向けの口だけが連絡先を返す。列を分けた以上、
	// 「公開側は空」「アカウント側は入っている」の両方を実際の SELECT で確かめる。
	t.Run("公開情報の取得は連絡先を返さない", func(t *testing.T) {
		// model.User は Email フィールドを持たないので「値が空か」では確かめられない
		// （持てないことは型が保証している）。ここで見張るのは、公開列で SELECT した
		// 行がちゃんと読めること＝列と Scan の数がずれていないことのほう。
		// 連絡先が出ないことそのものは TestUserColumnProjectionsSeparateEmail が列で固定する。
		u, err := userRepo.GetUserByID(ctx, creds.ID)
		if err != nil || u == nil {
			t.Fatalf("GetUserByID = %+v, %v", u, err)
		}
		if u.AccountID != "taro" || u.Name != "太郎" {
			t.Fatalf("公開情報が読めていない: %+v", u)
		}
	})

	t.Run("本人・管理者向けの取得は連絡先を返す", func(t *testing.T) {
		a, err := userRepo.GetUserAccountByID(ctx, creds.ID)
		if err != nil || a == nil {
			t.Fatalf("GetUserAccountByID = %+v, %v", a, err)
		}
		if a.Email != "taro@example.com" {
			t.Errorf("GetUserAccountByID の Email = %q, want %q", a.Email, "taro@example.com")
		}
		if a.AccountID != "taro" {
			t.Errorf("GetUserAccountByID の AccountID = %q, want taro（Scan の並びがずれている）", a.AccountID)
		}

		as, err := userRepo.GetUserAccountsByIDs(ctx, []int64{creds.ID})
		if err != nil || len(as) != 1 || as[0].Email != "taro@example.com" {
			t.Fatalf("GetUserAccountsByIDs = %+v, %v", as, err)
		}

		list, _, err := userRepo.ListUserAccounts(ctx, repository.PageQuery{Limit: 10})
		if err != nil || len(list) != 1 || list[0].Email != "taro@example.com" {
			t.Fatalf("ListUserAccounts = %+v, %v", list, err)
		}

		found, _, err := userRepo.SearchUserAccountsByKeyword(ctx, "太郎", repository.PageQuery{Limit: 10})
		if err != nil || len(found) != 1 || found[0].Email != "taro@example.com" {
			t.Fatalf("SearchUserAccountsByKeyword = %+v, %v", found, err)
		}
	})

	// 認証情報の口だけがハッシュを返す。
	t.Run("認証情報の取得はハッシュを返す", func(t *testing.T) {
		got, err := userRepo.FindCredentialsByEmail(ctx, "taro@example.com")
		if err != nil || got == nil {
			t.Fatalf("FindCredentialsByEmail = %+v, %v", got, err)
		}
		if got.HashedPassword != hash {
			t.Errorf("HashedPassword = %q, want %q", got.HashedPassword, hash)
		}
		byID, err := userRepo.GetCredentialsByID(ctx, creds.ID)
		if err != nil || byID == nil || byID.HashedPassword != hash {
			t.Fatalf("GetCredentialsByID = %+v, %v", byID, err)
		}
	})

	// 公開情報だけの UPDATE がハッシュを壊さない。
	// 分離前は UpdateUser が hashed_password も SET していたので、ハッシュを
	// 読んでいない経路（凍結・プロフィール更新）が保存すると空文字で潰れる形だった。
	t.Run("公開列だけの更新はハッシュを壊さない", func(t *testing.T) {
		u, err := userRepo.GetUserByID(ctx, creds.ID)
		if err != nil || u == nil {
			t.Fatalf("GetUserByID: %+v, %v", u, err)
		}
		u.Status = model.UserStatusFrozen
		if err := userRepo.UpdateUser(ctx, u); err != nil {
			t.Fatalf("UpdateUser: %v", err)
		}

		after, err := userRepo.GetCredentialsByID(ctx, creds.ID)
		if err != nil || after == nil {
			t.Fatalf("GetCredentialsByID: %+v, %v", after, err)
		}
		if after.HashedPassword != hash {
			t.Fatalf("公開列の更新でハッシュが壊れた: %q (want %q)", after.HashedPassword, hash)
		}
		// email も同じ理屈で壊れてはいけない。model.User は Email を持たないので、
		// UpdateUser が email も SET していたら空文字で潰れる。
		if after.Email != "taro@example.com" {
			t.Fatalf("公開列の更新で連絡先が壊れた: %q (want %q)", after.Email, "taro@example.com")
		}
		if after.Status != model.UserStatusFrozen {
			t.Errorf("status = %q, want frozen", after.Status)
		}
	})
}
