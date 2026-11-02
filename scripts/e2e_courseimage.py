"""授業内チャットの画像の一元管理（管理画面の一覧・削除）を、ローカルで動いているサーバーに対して通しで確かめる。

使い方(ローカルのサーバーが http://localhost:8080 で動いている状態で):

    DISABLE_USER_VALIDATION=true docker compose up -d app   # テスト用ユーザーをメール認証なしで作るため
    python3 scripts/e2e_courseimage.py
    docker compose up -d app                                # 終わったら通常の設定に戻す

やること: 利用者aがメッセージ・質問・回答に画像を付けて投稿 → 管理者が一覧で見られる(授業・種別で絞れる)→
一般ユーザーは見られない・消せない → 管理者が削除するとファイルも消える、を確認する。最後に "ALL OK" が出れば成功。
(画像のアップロードに、ローカルの MinIO が要る)

注意:
- ローカル開発用DBにテスト用ユーザー(@example.test)・メッセージ・リアクションが残る。
  確認が終わったら消してよい(DELETE FROM messages/users WHERE ... @example.test)。
- 管理者の認証情報は .env（SPACE_ENV_FILE で変更可）の INIT_ADMIN_* を読む(授業一覧の取得だけに使う)。
- 購読の確認に websocket-client(pip install websocket-client)が要る。
"""
import json, os, secrets, sys, time, urllib.error, urllib.request

URL = "http://localhost:8080/query"

def gql(query, variables=None, token=None):
    req = urllib.request.Request(URL, data=json.dumps({"query": query, "variables": variables or {}}).encode(),
                                 headers={"Content-Type": "application/json", **({"Authorization": f"Bearer {token}"} if token else {})})
    # サーバーのレート制限(429)に当たったら、少し待って最大5回やり直す。
    for attempt in range(6):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code != 429 or attempt == 5:
                raise
            time.sleep(2 + attempt * 2)

def ok(d):
    assert "errors" not in d, d["errors"]
    return d["data"]

def err(d):
    assert "errors" in d, f"expected an error, got {d}"
    return d["errors"][0]["message"]

import base64
PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

def via_localhost(url, headers=None):
    # 署名やURLの host は host.docker.internal だが、Mac からは名前が引けないことがある。
    # 接続先だけ localhost にし、Host ヘッダーは元のままにする（署名が合う）。
    from urllib.parse import urlsplit
    host = urlsplit(url).netloc
    return url.replace(host, "localhost:" + host.split(":")[-1], 1), {**(headers or {}), "Host": host}

def upload(token, field="presignedMediaUploadUrl"):
    # メッセージの添付だけ非公開の置き場に入る（presignedMessageMediaUploadUrl）。質問・回答は公開側。
    d = ok(gql("query{%s(contentType:\"image/png\"){uploadUrl objectKey}}" % field, token=token))[field]
    u, h = via_localhost(d["uploadUrl"], {"Content-Type": "image/png"})
    req = urllib.request.Request(u, data=PNG, method="PUT", headers=h)
    urllib.request.urlopen(req, timeout=30).read()
    return {"objectKey": d["objectKey"], "contentType": "image/png", "width": 1, "height": 1}

run = secrets.token_hex(3)
pw = "T" + secrets.token_urlsafe(12) + "1a"
acct = f"img{run}a"
ok(gql("mutation($i:CreateUserInput!){createUser(input:$i){ID}}",
       {"i": {"accountID": acct, "name": "画像試験a", "email": f"{acct}@example.test", "password": pw, "otp": "000000"}}))
ta = ok(gql("mutation($i:LoginInput!){loginUser(input:$i){token}}", {"i": {"email": f"{acct}@example.test", "password": pw}}))["loginUser"]["token"]
env = dict(l.strip().split("=", 1) for l in open(os.environ.get("SPACE_ENV_FILE", ".env"), encoding="utf-8") if "=" in l and not l.lstrip().startswith("#"))
adm = ok(gql("mutation($i:LoginInput!){loginAdministrator(input:$i){token}}",
             {"i": {"email": env["INIT_ADMIN_EMAIL"], "password": env["INIT_ADMIN_PASSWORD"]}}))["loginAdministrator"]["token"]
cur = ok(gql("query{currentSemester{year semester}}", token=adm))["currentSemester"]
cs = ok(gql("query($y:Int,$s:String){adminListCourses(year:$y,semester:$s,dayOfWeek:\"月\",limit:5){items{ID roomID courseName}}}",
            {"y": cur["year"], "s": cur["semester"]}, adm))["adminListCourses"]["items"]
course = cs[0]
ok(gql("mutation($c:ID!){registerTimetableEntry(courseID:$c){ID}}", {"c": course["ID"]}, ta))
print(f"1. テスト用ユーザーを作り、授業『{course['courseName']}』を履修 OK")

room = course["roomID"]
ok(gql("mutation($r:ID!,$m:[MediaUploadInput!]){sendMessage(roomID:$r,content:\"画像つき\",mediaInputs:$m){ID}}", {"r": room, "m": [upload(ta, "presignedMessageMediaUploadUrl")]}, ta))
q = ok(gql("mutation($r:ID!,$m:[MediaUploadInput!]){createQuestion(roomID:$r,body:\"画像つき質問\",mediaInputs:$m){ID}}", {"r": room, "m": [upload(ta)]}, ta))["createQuestion"]
ok(gql("mutation($i:ID!,$m:[MediaUploadInput!]){answerQuestion(questionID:$i,body:\"画像つき回答\",mediaInputs:$m){ID}}", {"i": q["ID"], "m": [upload(ta)]}, ta))
print("2. メッセージ・質問・回答に画像を付けて投稿 OK")

L = "query($r:ID,$s:CourseChatImageSource,$l:Int,$o:Int){adminListCourseChatImages(roomID:$r,source:$s,limit:$l,offset:$o){total items{ID source url width height roomID courseName uploaderName uploaderAccountID targetDeleted createdAt}}}"
mine = lambda items: [i for i in items if i["uploaderAccountID"] == acct]
page = ok(gql(L, {"r": room, "l": 50}, adm))["adminListCourseChatImages"]
got = mine(page["items"])
assert sorted(i["source"] for i in got) == ["ANSWER", "MESSAGE", "QUESTION"], got
assert all(i["courseName"] == course["courseName"] and i["roomID"] == room and i["width"] == 1 for i in got), got
print(f"3. 管理者の一覧に3種類(MESSAGE/QUESTION/ANSWER)の画像が出る（授業『{course['courseName']}』の合計 {page['total']} 枚）OK")
only_q = ok(gql(L, {"r": room, "s": "QUESTION", "l": 50}, adm))["adminListCourseChatImages"]
assert all(i["source"] == "QUESTION" for i in only_q["items"]) and len(mine(only_q["items"])) == 1
first = ok(gql(L, {"l": 1, "o": 0}, adm))["adminListCourseChatImages"]
assert len(first["items"]) == 1 and first["total"] >= 3
print("   - 種別で絞れる・limit/offset で区切れる OK")

print("4. 一般ユーザー:", err(gql(L, {"r": room}, ta)))
target = [i for i in got if i["source"] == "MESSAGE"][0]
D = "mutation($i:ID!){adminDeleteCourseChatImage(mediaID:$i)}"
print("   - 削除もできない:", err(gql(D, {"i": target["ID"]}, ta)))

def fetch_status(url):
    try:
        u, h = via_localhost(url)
        return urllib.request.urlopen(urllib.request.Request(u, headers=h), timeout=15).status
    except urllib.error.HTTPError as e:
        return e.code
assert fetch_status(target["url"]) == 200
assert ok(gql(D, {"i": target["ID"]}, adm))["adminDeleteCourseChatImage"] is True
assert fetch_status(target["url"]) == 404, "ファイルも消えているはず"
after = mine(ok(gql(L, {"r": room, "l": 50}, adm))["adminListCourseChatImages"]["items"])
assert len(after) == 2 and all(i["ID"] != target["ID"] for i in after)
print("5. 管理者が削除 → 一覧から消え、ファイルの公開URLも 404 になる OK")
print("   - もう一度消そうとすると:", err(gql(D, {"i": target["ID"]}, adm)))
print("   - 存在しない/授業の画像でないID:", err(gql(D, {"i": "bWVkaWE6OTk5OTk5OTpYWFhYWFhYWFhYWFhYWFha"}, adm)))
print("ALL OK")
