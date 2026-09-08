# チャット負荷テスト (k6)

授業チャットの負荷テストケース(項番77: 大量データ時のページング性能、項番78: 同時接続時のリアルタイム配信)を [k6](https://k6.io/) で実行するスクリプト。

## 前提

- [k6 CLI](https://k6.io/docs/get-started/installation/) がインストール済みであること。
- 対象サーバーが起動していること(`make test-integration-up` 等でDBも含めて用意する)。
- テスト対象の授業チャットに登録済みの学生アカウント(メール/パスワード)を用意すること。
- その授業チャットルームの opaque room id (`ROOM_ID`) を控えておくこと。ブラウザで対象授業チャットを開き、devtools の Network タブで `messages` クエリのリクエスト変数 `roomID` を確認するのが手早い。

## 実行方法

```sh
# 項番77: 大量データ時の検索・ページング性能
k6 run load/chat-history-load.js \
  --env BASE_URL=http://localhost:8080 \
  --env LOAD_TEST_EMAIL=student@example.com \
  --env LOAD_TEST_PASSWORD=xxxxx \
  --env ROOM_ID=<opaque room id> \
  --env SEED=true --env SEED_COUNT=10000   # 初回のみ、データ投入も行う場合

# 項番78: 50〜100人同時接続時のリアルタイム配信
k6 run load/chat-realtime-load.js \
  --env BASE_URL=http://localhost:8080 \
  --env LOAD_TEST_EMAIL=student@example.com \
  --env LOAD_TEST_PASSWORD=xxxxx \
  --env ROOM_ID=<opaque room id> \
  --env VUS=100
```

`make test-load` でも同様に(環境変数を事前に export した上で)まとめて実行できる。

## 注意

- `SEED=true` で10,000件投入する場合、投入自体がAPI呼び出し1万回分の負荷になるため時間がかかる(数分〜)。ステージング等の使い捨て環境で実行すること。本番環境には実行しない。
- `chat-realtime-load.js` は各VUが「自分宛のメッセージを送って自分の購読で受信するまでの時間」を配信遅延の指標として計測する。同一ルームに多数のVUが同時購読するため、ブロードキャスト配信自体の負荷は現実的に再現される。
- しきい値 (`thresholds`) はデフォルト値を仮置きしているため、実際のインフラ規模に応じて `--env` や `options.thresholds` を調整すること。
