"""質問・回答への追記を、ローカルで動いているサーバーに対して通しで確かめる。

使い方(ローカルのサーバーが http://localhost:8080 で動いている状態で):

    DISABLE_USER_VALIDATION=true docker compose up -d app   # テスト用ユーザーをメール認証なしで作るため
    python3 scripts/e2e_addendum.py
    docker compose up -d app                                # 終わったら通常の設定に戻す

やること: 質問者a・回答者b・同じ授業のcを作る → aが質問、bが回答 → aが質問に追記（元の本文は変わらない）、
bが回答に追記 → 本人以外は追記できない・上限・NGワード・通知の宛先を確認する。最後に "ALL OK" が出れば成功。

注意:
- ローカル開発用DBにテスト用ユーザー(@example.test)と投稿が残る。
  確認が終わったら消してよい(DELETE FROM messages/users WHERE ... @example.test)。
- 管理者の認証情報は .env（SPACE_ENV_FILE で変更可）の INIT_ADMIN_* を読む(授業一覧の取得だけに使う)。
"""
import secrets

from e2e_common import err, gql, ok, read_env

run = secrets.token_hex(3)
pw = "T" + secrets.token_urlsafe(12) + "1a"
users = {}
for name in ("a", "b", "c"):
    acct = f"add{run}{name}"
    ok(gql("mutation($i:CreateUserInput!){createUser(input:$i){ID}}",
           {"i": {"accountID": acct, "name": f"追記試験{name}", "email": f"{acct}@example.test", "password": pw, "otp": "000000"}}))
    d = ok(gql("mutation($i:LoginInput!){loginUser(input:$i){token}}", {"i": {"email": f"{acct}@example.test", "password": pw}}))
    users[name] = d["loginUser"]["token"]
print("1. テスト用ユーザー3人(a=質問者 b=回答者 c=第三者)を作成 OK")

env = read_env()
adm = ok(gql("mutation($i:LoginInput!){loginAdministrator(input:$i){token}}",
             {"i": {"email": env["INIT_ADMIN_EMAIL"], "password": env["INIT_ADMIN_PASSWORD"]}}))["loginAdministrator"]["token"]
cur = ok(gql("query{currentSemester{year semester}}", token=adm))["currentSemester"]
cs = ok(gql("query($y:Int,$s:String){adminListCourses(year:$y,semester:$s,dayOfWeek:\"月\",limit:5){items{ID roomID courseName}}}",
            {"y": cur["year"], "s": cur["semester"]}, adm))["adminListCourses"]["items"]
course = cs[0]
for name in ("a", "b", "c"):
    ok(gql("mutation($c:ID!){registerTimetableEntry(courseID:$c){ID}}", {"c": course["ID"]}, users[name]))
print(f"2. 3人が授業『{course['courseName']}』を履修登録 OK")

q = ok(gql("mutation($r:ID!,$b:String!){createQuestion(roomID:$r,body:$b){ID body addenda{ID}}}", {"r": course["roomID"], "b": "レポートの締切はいつですか"}, users["a"]))["createQuestion"]
assert q["addenda"] == [], q
ans = ok(gql("mutation($i:ID!,$b:String!){answerQuestion(questionID:$i,body:$b){ID addenda{ID}}}", {"i": q["ID"], "b": "来週の金曜です"}, users["b"]))["answerQuestion"]
assert ans["addenda"] == [], ans
print("3. a が質問、b が回答。追記は空配列 OK")

AQ = "mutation($i:ID!,$b:String!){addQuestionAddendum(questionID:$i,body:$b){ID body addenda{ID body createdAt}}}"
r = ok(gql(AQ, {"i": q["ID"], "b": "  追記: 提出は紙ですか？  "}, users["a"]))["addQuestionAddendum"]
assert r["body"] == "レポートの締切はいつですか", r
assert [x["body"] for x in r["addenda"]] == ["追記: 提出は紙ですか？"], r
print("4. 質問者 a が追記。元の本文は変わらず、追記は別の枠(前後の空白は取る) OK")

print("5. 追記できない人・内容:")
print("   - 回答者 b が質問に追記:", err(gql(AQ, {"i": q["ID"], "b": "x"}, users["b"])))
print("   - 第三者 c が質問に追記:", err(gql(AQ, {"i": q["ID"], "b": "x"}, users["c"])))
print("   - 空の追記:", err(gql(AQ, {"i": q["ID"], "b": "   "}, users["a"])))
print("   - 1001文字:", err(gql(AQ, {"i": q["ID"], "b": "あ" * 1001}, users["a"])))
print("   - 未ログイン:", err(gql(AQ, {"i": q["ID"], "b": "x"})))

QN = "query{myNotifications(limit:50){items{ID type message targetType targetID targetQuestion{ID roomID} actor{ID}}}}"

def notes(user, typ):
    # myNotifications の type 絞り込みは actorID と一緒のときだけ効くので、ここで絞る。
    return [n for n in ok(gql(QN, token=users[user]))["myNotifications"]["items"] if n["type"] == typ]
nb = notes("b", "question_addendum")
assert len(nb) == 1 and nb[0]["actor"] is None and nb[0]["targetType"] == "question", nb
assert nb[0]["targetQuestion"]["ID"] == q["ID"] and nb[0]["targetQuestion"]["roomID"] == course["roomID"], nb
print("6. 回答している b に『質問に追記』の通知が1件(追記した人は載らない・遷移先の質問が引ける):", nb[0]["message"])
for n in ("a", "c"):
    assert notes(n, "question_addendum") == [], n
print("   - 質問者本人 a と無関係の c には通知が出ない OK")

# 追記は他の人が質問一覧を引いても見える
lst = ok(gql("query($r:ID!){questions(roomID:$r,limit:20){items{ID body addenda{body}}}}", {"r": course["roomID"]}, users["c"]))["questions"]["items"]
mine = [x for x in lst if x["ID"] == q["ID"]][0]
assert [x["body"] for x in mine["addenda"]] == ["追記: 提出は紙ですか？"], mine
print("7. 第三者 c が質問一覧を見ると、追記が付いている OK")

AA = "mutation($i:ID!,$b:String!){addAnswerAddendum(answerID:$i,body:$b){ID body addenda{ID body}likeCount}}"
r = ok(gql(AA, {"i": ans["ID"], "b": "補足: 変更になる場合は連絡します"}, users["b"]))["addAnswerAddendum"]
assert r["body"] == "来週の金曜です" and [x["body"] for x in r["addenda"]] == ["補足: 変更になる場合は連絡します"], r
print("8. 回答者 b が回答に追記。元の本文は変わらない OK")
print("   - 質問者 a が回答に追記:", err(gql(AA, {"i": ans["ID"], "b": "x"}, users["a"])))
na = notes("a", "answer_addendum")
assert len(na) == 1 and na[0]["actor"] is None and na[0]["targetQuestion"]["ID"] == q["ID"], na
print("9. 質問者 a に『回答に追記』の通知が1件:", na[0]["message"])
nb2 = notes("b", "answer_addendum")
assert nb2 == [], nb2
print("   - 追記した b 本人には通知が出ない OK")

# NGワード
word = f"ngadd{run}"
ng = ok(gql("mutation($p:String!){adminCreateNGWord(pattern:$p,matchType:WORD){ID}}", {"p": word}, adm))["adminCreateNGWord"]
print("10. 追記もNGワードで止まる:", err(gql(AQ, {"i": q["ID"], "b": f"{word} です"}, users["a"])))
ok(gql("mutation($i:ID!){adminDeleteNGWord(id:$i)}", {"i": ng["ID"]}, adm))

# 上限(10件)
for n in range(2, 11):
    ok(gql(AQ, {"i": q["ID"], "b": f"追記{n}"}, users["a"]))
print("11. 11件目は上限で止まる:", err(gql(AQ, {"i": q["ID"], "b": "11件目"}, users["a"])))
print("ALL OK")
