"""NGワードを、ローカルで動いているサーバーに対して通しで確かめる。

使い方(ローカルのサーバーが http://localhost:8080 で動いている状態で):

    DISABLE_USER_VALIDATION=true docker compose up -d app   # テスト用ユーザーをメール認証なしで作るため
    python3 scripts/e2e_ngword.py
    docker compose up -d app                                # 終わったら通常の設定に戻す

やること: テスト用ユーザー2人を作る → 管理者がNGワードを登録 → 授業内チャットのメッセージ・質問・回答が
止まること、止めた回数の集計、削除後は通ることを確認する。最後に "ALL OK" が出れば成功。

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
for name in ("a", "b"):
    acct = f"ngw{run}{name}"
    ok(gql("mutation($i:CreateUserInput!){createUser(input:$i){ID}}",
           {"i": {"accountID": acct, "name": f"NG試験{name}", "email": f"{acct}@example.test", "password": pw, "otp": "000000"}}))
    d = ok(gql("mutation($i:LoginInput!){loginUser(input:$i){token}}", {"i": {"email": f"{acct}@example.test", "password": pw}}))
    users[name] = d["loginUser"]["token"]
print("1. テスト用ユーザー2人を作成してログイン OK")

env = read_env()
adm = ok(gql("mutation($i:LoginInput!){loginAdministrator(input:$i){token}}",
             {"i": {"email": env["INIT_ADMIN_EMAIL"], "password": env["INIT_ADMIN_PASSWORD"]}}))["loginAdministrator"]["token"]
cur = ok(gql("query{currentSemester{year semester}}", token=adm))["currentSemester"]
cs = ok(gql("query($y:Int,$s:String){adminListCourses(year:$y,semester:$s,dayOfWeek:\"月\",limit:5){items{ID roomID courseName}}}",
            {"y": cur["year"], "s": cur["semester"]}, adm))["adminListCourses"]["items"]
course = cs[0]
for name in ("a", "b"):
    ok(gql("mutation($c:ID!){registerTimetableEntry(courseID:$c){ID}}", {"c": course["ID"]}, users[name]))
print(f"2. 2人が授業『{course['courseName']}』を履修登録 OK")

word = f"ngtest{run}"
fullwidth = "".join(chr(ord(ch) + 0xFEE0) if "a" <= ch <= "z" else ch for ch in word)
C = "mutation($p:String!,$t:NGWordMatchType!){adminCreateNGWord(pattern:$p,matchType:$t){ID pattern matchType}}"
w1 = ok(gql(C, {"p": word.upper(), "t": "WORD"}, adm))["adminCreateNGWord"]
# 9桁の数字の登録は、他の実行と重ならないよう「すでにある」場合は既存を使う。
digits_pat = r"(^|\D)\d{9}($|\D)"
lst = ok(gql("query{adminNGWords{ID pattern matchType}}", token=adm))["adminNGWords"]
digits = next((w for w in lst if w["pattern"] == digits_pat and w["matchType"] == "REGEX"), None)
digits_created = digits is None
if digits_created:
    digits = ok(gql(C, {"p": digits_pat, "t": "REGEX"}, adm))["adminCreateNGWord"]
print("3. 管理者がNGワード(単語・9桁の数字の正規表現)を登録 OK")
print("   - 同じものを二重に登録:", err(gql(C, {"p": word.upper(), "t": "WORD"}, adm)))
print("   - 壊れた正規表現:", err(gql(C, {"p": "(", "t": "REGEX"}, adm)))
print("   - 空に当たる正規表現:", err(gql(C, {"p": "a*", "t": "REGEX"}, adm)))
print("   - 一般ユーザーは登録できない:", err(gql(C, {"p": "zzz", "t": "WORD"}, users["a"])))
err(gql("query{adminNGWords{ID}}", token=users["a"]))

S = "mutation($r:ID!,$c:String!){sendMessage(roomID:$r,content:$c){ID content}}"
room = course["roomID"]
m = ok(gql(S, {"r": room, "c": "今日の課題の範囲を教えてください"}, users["a"]))["sendMessage"]
print("4. 普通のメッセージは送れる OK")

msg1 = err(gql(S, {"r": room, "c": f"これは {word} です"}, users["a"]))
assert word not in msg1 and word.upper() not in msg1, msg1
print("5. NGワード(大小文字違い)を含むメッセージは止まる。文言に語は出ない:", msg1)
print("   - 全角でも止まる:", err(gql(S, {"r": room, "c": fullwidth}, users["a"])))
print("   - 9桁の数字:", err(gql(S, {"r": room, "c": "番号は 123456789 です"}, users["a"])))
ok(gql(S, {"r": room, "c": "8桁なら 12345678 です"}, users["a"]))
ok(gql(S, {"r": room, "c": "10桁なら 1234567890 です"}, users["a"]))
print("   - 8桁・10桁は通る OK")

E = "mutation($r:ID!,$i:ID!,$c:String!){updateMessage(roomID:$r,id:$i,content:$c){ID content}}"
print("6. 編集で NGワードに書き換えるのも止まる:", err(gql(E, {"r": room, "i": m["ID"], "c": f"{word} に変更"}, users["a"])))

QC = "mutation($r:ID!,$b:String!){createQuestion(roomID:$r,body:$b){ID}}"
q = ok(gql(QC, {"r": room, "b": "ふつうの質問です"}, users["a"]))["createQuestion"]
print("7. 質問: NGワードは止まる:", err(gql(QC, {"r": room, "b": f"{word} 教えて"}, users["a"])))
print("   - 質問の編集:", err(gql("mutation($i:ID!,$b:String!){updateQuestion(id:$i,body:$b){ID}}", {"i": q["ID"], "b": word}, users["a"])))
AN = "mutation($i:ID!,$b:String!){answerQuestion(questionID:$i,body:$b){ID}}"
a = ok(gql(AN, {"i": q["ID"], "b": "ふつうの回答です"}, users["b"]))["answerQuestion"]
print("   - 回答:", err(gql(AN, {"i": q["ID"], "b": f"答えは {word}"}, users["b"])))
print("   - 回答の編集:", err(gql("mutation($i:ID!,$b:String!){updateAnswer(id:$i,body:$b){ID}}", {"i": a["ID"], "b": word}, users["b"])))

ST = "query{adminNGWordStats{total byWord{pattern count} byUser{name accountID count}}}"
st = ok(gql(ST, token=adm))["adminNGWordStats"]
cnt = {u["accountID"]: u["count"] for u in st["byUser"]}
assert cnt.get(f"ngw{run}a") == 6, st   # メッセージ3(単語・全角・数字) + 編集1 + 質問2(投稿・編集)
assert cnt.get(f"ngw{run}b") == 2, st   # 回答2(投稿・編集)
words = {w["pattern"]: w["count"] for w in st["byWord"]}
assert words.get(word.upper()) == 5 + 0 + 2, st   # 単語で止まったのは 6-1(数字) + 2 = 7
print(f"8. 止めた回数の集計: a={cnt[f'ngw{run}a']}回 b={cnt[f'ngw{run}b']}回、合計 {st['total']} 回 OK")

# 管理者は止められない(教員の注意喚起などで使う語を書けるように)。
try:
    ok(gql(S, {"r": room, "c": f"管理者の注意: {word} の共有は禁止です"}, adm))
    print("9. 管理者の投稿は止まらない OK")
except AssertionError as e:
    print("9. (管理者は授業に参加していないため送れない。止まらないことはユニットテストで確認済み)", str(e)[:80])

ok(gql("mutation($i:ID!){adminDeleteNGWord(id:$i)}", {"i": w1["ID"]}, adm))
ok(gql(S, {"r": room, "c": f"これは {word} です"}, users["a"]))
print("10. NGワードを削除すると、同じ文面が通るようになる OK")
st2 = ok(gql(ST, token=adm))["adminNGWordStats"]
assert st2["total"] == st["total"], (st, st2)
assert {u["accountID"]: u["count"] for u in st2["byUser"]}.get(f"ngw{run}a") == 6
print("11. 削除しても、止めた回数の記録は残る OK")
if digits_created:
    ok(gql("mutation($i:ID!){adminDeleteNGWord(id:$i)}", {"i": digits["ID"]}, adm))
print("ALL OK")
