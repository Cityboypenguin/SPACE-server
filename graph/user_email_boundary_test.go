package graph

import (
	"strings"
	"testing"
)

// (A) の要。「他人を見せるフィールドが email を持つ型を返していないか」を
// スキーマそのもので見張る。
//
// 元の指摘はこうだった: searchUsers は一般認証ユーザーから使えるのに、
// User.email が非 null の公開フィールドだったので、他人のメールアドレスが取れていた。
//
// 直し方は「email を持つ型（UserAccount）と持たない型（User）に分け、
// 本人・管理者しか辿れない口だけが UserAccount を返す」。この分け方は、
// 新しくユーザーを返すフィールドを足す人が何も考えないと崩れる（うっかり
// UserAccount を返す口を公開してしまう）ので、ここで許可リストとして固定する。
//
// テストが落ちたら、まず「そのフィールドは本当に本人か管理者しか辿れないか」を
// リゾルバで確かめること。確かめられないなら User を返すように直す。
// 確かめられるなら、この許可リストへ理由付きで足す。

// userAccountAllowedFields は UserAccount（email を持つ型）を返してよい場所。
// キーは "<型>.<フィールド>"。値はなぜ許されるか。
var userAccountAllowedFields = map[string]string{
	"Query.me":                 "本人。claims.ID しか見ない",
	"Query.users":              "管理者専用（requireAdminAuth）",
	"Query.getUserByID":        "管理者専用（requireAdminAuth）",
	"Query.adminSearchUsers":   "管理者専用（requireAdminAuth）",
	"Mutation.createUser":      "本人。いま自分で入力したメールアドレスを返すだけ",
	"Mutation.updateUser":      "本人。更新対象は claims.ID 固定",
	"Mutation.adminUpdateUser": "管理者専用（requireAdminAuth）",
	"UserAuthPayload.user":     "ログイン／トークン更新をした本人",
	"TermsConsentRecord.user":  "adminListConsents（管理者専用）からしか辿れない",
	"UserAccountPage.items":    "UserAccount を返す口のページ型。中身も UserAccount",
}

// isUserAccountType は「連絡先を含むユーザー型」かどうか。UserAccountPage も含めるのは、
// ページ型を返すフィールドから items 経由で UserAccount に辿れるため。
func isUserAccountType(name string) bool {
	return name == "UserAccount" || name == "UserAccountPage"
}

func TestSchema_UserAccountIsOnlyReachableFromSelfOrAdminFields(t *testing.T) {
	schema := NewExecutableSchema(Config{}).Schema()

	var offenders []string
	for typeName, def := range schema.Types {
		// 内省用の型（__Schema など）は対象外。
		if strings.HasPrefix(typeName, "__") {
			continue
		}
		for _, field := range def.Fields {
			if strings.HasPrefix(field.Name, "__") {
				continue
			}
			// ast.Type.Name() は [T!]! のような皮を剥いだ名前付き型の名前を返す。
			returned := field.Type.Name()
			if !isUserAccountType(returned) {
				continue
			}
			key := typeName + "." + field.Name
			if _, ok := userAccountAllowedFields[key]; !ok {
				offenders = append(offenders, key+" -> "+returned)
			}
		}
	}

	if len(offenders) > 0 {
		t.Fatalf(
			"許可されていないフィールドが UserAccount（email を含む型）を返している:\n  %s\n\n"+
				"本人か管理者しか辿れないことをリゾルバで保証できないなら User を返すこと。"+
				"保証できるなら userAccountAllowedFields へ理由付きで足すこと。",
			strings.Join(offenders, "\n  "),
		)
	}

	// 許可リストが現実と乖離していないか（消えたフィールドが残っていないか）も見る。
	// 残っていると「守っているつもりの穴」になるので、ここで落とす。
	present := map[string]bool{}
	for typeName, def := range schema.Types {
		for _, field := range def.Fields {
			present[typeName+"."+field.Name] = true
		}
	}
	for key := range userAccountAllowedFields {
		if !present[key] {
			t.Errorf("許可リストに実在しないフィールドが残っている: %s", key)
		}
	}
}

// User（公開型）が email を持たないことを直接固定する。
// 上のテストは「UserAccount がどこから見えるか」を見張るもので、
// こちらは「公開型に連絡先を足し戻していないか」を見張るもの。
func TestSchema_PublicUserTypeHasNoEmail(t *testing.T) {
	schema := NewExecutableSchema(Config{}).Schema()

	userDef, ok := schema.Types["User"]
	if !ok {
		t.Fatal("User 型が見つからない")
	}
	for _, field := range userDef.Fields {
		if field.Name == "email" {
			t.Fatal("User に email が復活している。他人を見せる文脈で連絡先が出るので、" +
				"本人・管理者向けの UserAccount へ載せること")
		}
	}

	accountDef, ok := schema.Types["UserAccount"]
	if !ok {
		t.Fatal("UserAccount 型が見つからない")
	}
	var hasEmail bool
	for _, field := range accountDef.Fields {
		if field.Name == "email" {
			hasEmail = true
		}
	}
	if !hasEmail {
		t.Fatal("UserAccount に email が無い。本人・管理者向けの口が成り立たない")
	}
}
