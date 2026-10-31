"""メッセージへのリアクションを、ローカルで動いているサーバーに対して通しで確かめる。

使い方(ローカルのサーバーが http://localhost:8080 で動いている状態で):

    DISABLE_USER_VALIDATION=true docker compose up -d app   # テスト用ユーザーをメール認証なしで作るため
    python3 scripts/e2e_message_reaction.py
    docker compose up -d app                                # 終わったら通常の設定に戻す

やること: テスト用ユーザー3人を作る → 2人が同じ授業を履修 → メッセージを送る →
リアクションの付け外し・冪等性・拒否されるべき操作・購読(WebSocket)への配信を確認する。
最後に "ALL OK" が出れば成功。

注意:
- ローカル開発用DBにテスト用ユーザー(@example.test)・メッセージ・リアクションが残る。
  確認が終わったら消してよい(DELETE FROM messages/users WHERE ... @example.test)。
- 管理者の認証情報は server/.env の INIT_ADMIN_* を読む(授業一覧の取得だけに使う)。
- 購読の確認に websocket-client(pip install websocket-client)が要る。
"""
import json, secrets, sys, urllib.request

URL = "http://localhost:8080/query"

def gql(query, variables=None, token=None):
    req = urllib.request.Request(URL, data=json.dumps({"query": query, "variables": variables or {}}).encode(),
                                 headers={"Content-Type": "application/json", **({"Authorization": f"Bearer {token}"} if token else {})})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)

def ok(d):
    assert "errors" not in d, d["errors"]
    return d["data"]

def err(d):
    assert "errors" in d, f"expected an error, got {d}"
    return d["errors"][0]["message"]

run = secrets.token_hex(3)
pw = "T" + secrets.token_urlsafe(12) + "1a"   # 使い捨てのテスト用パスワード(出力しない)
users = {}
for name in ("a", "b", "c"):
    acct = f"rxn{run}{name}"
    ok(gql("mutation($i:CreateUserInput!){createUser(input:$i){ID}}",
           {"i": {"accountID": acct, "name": f"リアクション試験{name}", "email": f"{acct}@example.test", "password": pw, "otp": "000000"}}))
    d = ok(gql("mutation($i:LoginInput!){loginUser(input:$i){token}}", {"i": {"email": f"{acct}@example.test", "password": pw}}))
    users[name] = d["loginUser"]["token"]
print("1. テスト用ユーザー3人を作成してログイン OK")

# 学生用の授業検索(searchCourses)は dev_new の最新コミットで壊れている(別件)ため、
# 管理者の授業一覧から現学期の授業を2つ取る。管理者の認証情報は server/.env(ローカル開発用)から読む。
env = dict(l.strip().split("=", 1) for l in open("/Users/yuki.s/SPACE/server/.env", encoding="utf-8") if "=" in l and not l.lstrip().startswith("#"))
adm = ok(gql("mutation($i:LoginInput!){loginAdministrator(input:$i){token}}",
             {"i": {"email": env["INIT_ADMIN_EMAIL"], "password": env["INIT_ADMIN_PASSWORD"]}}))["loginAdministrator"]["token"]
cur = ok(gql("query{currentSemester{year semester}}", token=adm))["currentSemester"]
cs = ok(gql("query($y:Int,$s:String){adminListCourses(year:$y,semester:$s,dayOfWeek:\"月\",limit:5){items{ID roomID courseName}}}",
            {"y": cur["year"], "s": cur["semester"]}, adm))["adminListCourses"]["items"]
course, other_course = cs[0], cs[1]
for name in ("a", "b"):   # c は履修しない
    ok(gql("mutation($c:ID!){registerTimetableEntry(courseID:$c){ID}}", {"c": course["ID"]}, users[name]))
print(f"2. a と b が授業『{course['courseName']}』を履修登録 OK (c は未履修)")

msg = ok(gql("mutation($r:ID!){sendMessage(roomID:$r,content:\"リアクション試験\"){ID reactions{emoji count reactedByMe}}}",
             {"r": course["roomID"]}, users["a"]))["sendMessage"]
assert msg["reactions"] == [], msg
print("3. a がメッセージ送信。reactions は空配列 OK")

emojis = ok(gql("query{messageReactionEmojis}", token=users["b"]))["messageReactionEmojis"]
assert "stamp:thanks" in emojis and "stamp:ryokai" in emojis and "🖕" not in emojis, emojis
print(f"4. 許可リスト({len(emojis)}個、文字スタンプ)に stamp:thanks と stamp:ryokai がある OK")

R = "mutation($r:ID!,$m:ID!,$e:String!){%s(roomID:$r,messageID:$m,emoji:$e){ID reactions{emoji count reactedByMe}}}"
v = {"r": course["roomID"], "m": msg["ID"], "e": "stamp:thanks"}
r1 = ok(gql(R % "addMessageReaction", v, users["b"]))["addMessageReaction"]["reactions"]
assert r1 == [{"emoji": "stamp:thanks", "count": 1, "reactedByMe": True}], r1
print("5. b がスタンプを付ける → 1件・b から見て reactedByMe=true OK")

r2 = ok(gql(R % "addMessageReaction", v, users["b"]))["addMessageReaction"]["reactions"]
assert r2 == r1, r2
print("6. b がもう一度付ける → 変わらない(冪等) OK")

r3 = ok(gql(R % "addMessageReaction", v, users["a"]))["addMessageReaction"]["reactions"]
assert r3 == [{"emoji": "stamp:thanks", "count": 2, "reactedByMe": True}], r3
print("7. 投稿者本人 a も付けられる → 2件 OK")

Q = "query($r:ID!){messages(roomID:$r,limit:10){items{ID reactions{emoji count reactedByMe}}}}"
for name, expect in (("a", True), ("b", True), ("c", False)):
    items = ok(gql(Q, {"r": course["roomID"]}, users[name]))["messages"]["items"]
    got = [i for i in items if i["ID"] == msg["ID"]][0]["reactions"]
    assert got == [{"emoji": "stamp:thanks", "count": 2, "reactedByMe": expect}], (name, got)
print("8. 一覧では閲覧者ごとに reactedByMe が変わる(a=true b=true c=false)、数は全員2 OK")

print("9. 拒否されるべき操作:")
print("   - 許可リスト外の絵文字(🖕):", err(gql(R % "addMessageReaction", {**v, "e": "🖕"}, users["b"])))
print("   - 未履修の c が付ける:", err(gql(R % "addMessageReaction", v, users["c"])))
bad = {**v, "r": other_course["roomID"]}
print("   - 別の授業のroomIDを名乗る:", err(gql(R % "addMessageReaction", bad, users["a"])))
print("   - 未ログイン:", err(gql(R % "addMessageReaction", v)))

r4 = ok(gql(R % "removeMessageReaction", v, users["b"]))["removeMessageReaction"]["reactions"]
assert r4 == [{"emoji": "stamp:thanks", "count": 1, "reactedByMe": False}], r4
r5 = ok(gql(R % "removeMessageReaction", v, users["b"]))["removeMessageReaction"]["reactions"]
assert r5 == r4, r5
print("10. b が外す → 1件・reactedByMe=false。もう一度外しても変わらない(冪等) OK")

# --- スタンプだけのメッセージ ---
SM = "mutation($r:ID!,$c:String!,$s:String,$m:[ID!],$rp:ID){sendMessage(roomID:$r,content:$c,stampID:$s,mentionUserIDs:$m,replyToID:$rp){ID content stampID replyToID}}"
st = ok(gql(SM, {"r": course["roomID"], "c": "", "s": "stamp:ryokai"}, users["a"]))["sendMessage"]
assert st["stampID"] == "stamp:ryokai" and st["content"] == "", st
print("12. a がスタンプ(了解です)だけを1件のメッセージとして送信 OK")

items = ok(gql("query($r:ID!){messages(roomID:$r,limit:20){items{ID content stampID}}}", {"r": course["roomID"]}, users["b"]))["messages"]["items"]
got = [i for i in items if i["ID"] == st["ID"]]
assert got and got[0]["stampID"] == "stamp:ryokai", got
normal = [i for i in items if i["ID"] == msg["ID"]][0]
assert normal["stampID"] is None, normal
print("13. 一覧に stampID つきで出る。通常のメッセージは stampID=null OK")

print("14. 拒否されるべきスタンプの送り方:")
print("   - 本文つき:", err(gql(SM, {"r": course["roomID"], "c": "こんにちは", "s": "stamp:ryokai"}, users["a"])))
print("   - 許可リスト外のID:", err(gql(SM, {"r": course["roomID"], "c": "", "s": "stamp:nothing"}, users["a"])))
print("   - 絵文字そのもの:", err(gql(SM, {"r": course["roomID"], "c": "", "s": "👍"}, users["a"])))
print("   - 未履修の c:", err(gql(SM, {"r": course["roomID"], "c": "", "s": "stamp:ryokai"}, users["c"])))
print("   - 編集:", err(gql("mutation($r:ID!,$i:ID!){updateMessage(roomID:$r,id:$i,content:\"x\"){ID}}", {"r": course["roomID"], "i": st["ID"]}, users["a"])))

rp = ok(gql(SM, {"r": course["roomID"], "c": "返信です", "s": None, "rp": st["ID"]}, users["b"]))["sendMessage"]
assert rp["replyToID"] == st["ID"], rp
rr = ok(gql("query($r:ID!){messages(roomID:$r,limit:20){items{ID replyTo{ID stampID content}}}}", {"r": course["roomID"]}, users["b"]))["messages"]["items"]
rep = [i for i in rr if i["ID"] == rp["ID"]][0]["replyTo"]
assert rep["stampID"] == "stamp:ryokai", rep
print("15. スタンプのメッセージへ返信すると、返信先に stampID が付いて出る OK")

added = ok(gql(R % "addMessageReaction", {"r": course["roomID"], "m": st["ID"], "e": "stamp:thanks"}, users["b"]))["addMessageReaction"]["reactions"]
assert added == [{"emoji": "stamp:thanks", "count": 1, "reactedByMe": True}], added
print("16. スタンプのメッセージにも、リアクションを付けられる OK")

dl = ok(gql("mutation($r:ID!,$i:ID!){deleteMessage(roomID:$r,id:$i)}", {"r": course["roomID"], "i": st["ID"]}, users["a"]))["deleteMessage"]
assert dl is True
print("17. スタンプのメッセージは、本人が削除できる OK")

# --- リアルタイム配信(購読)。b の接続は同じメッセージの更新を続けて受け取る ---
import websocket
ws = websocket.create_connection("ws://localhost:8080/query", subprotocols=["graphql-transport-ws"], origin="http://localhost:5173", timeout=10)
ws.send(json.dumps({"type": "connection_init", "payload": {"Authorization": f"Bearer {users['b']}"}}))
while json.loads(ws.recv())["type"] != "connection_ack":
    pass
ws.send(json.dumps({"id": "1", "type": "subscribe", "payload": {
    "query": "subscription($r:ID!){messageUpdated(roomID:$r){ID reactions{emoji count reactedByMe}}}", "variables": {"r": course["roomID"]}}}))
import time; time.sleep(1)

def next_event():
    while True:
        m = json.loads(ws.recv())
        if m["type"] == "next":
            return m["payload"]["data"]["messageUpdated"]["reactions"]

# 現在: b は外した後で、a だけが付けている(1件・b は未リアクション)
ok(gql(R % "removeMessageReaction", v, users["a"]))   # a も外す → 0件
assert next_event() == [], "a が外す → 0件"
ok(gql(R % "addMessageReaction", v, users["a"]))
e1 = next_event()
assert e1 == [{"emoji": "stamp:thanks", "count": 1, "reactedByMe": False}], e1
ok(gql(R % "addMessageReaction", v, users["b"]))      # b 自身が付ける
e2 = next_event()
assert e2 == [{"emoji": "stamp:thanks", "count": 2, "reactedByMe": True}], e2   # 古いキャッシュなら count=1 / false のまま
ok(gql(R % "removeMessageReaction", v, users["a"]))
e3 = next_event()
assert e3 == [{"emoji": "stamp:thanks", "count": 1, "reactedByMe": True}], e3
ws.close()
print("11. 購読中の b に、同じメッセージの更新が毎回最新の集計で届く (1件/false → 2件/true → 1件/true) OK")
print("ALL OK")
