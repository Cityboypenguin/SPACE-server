// 項番77: 大量データ(1万件規模)の授業チャットに対する検索・ページングが
// 許容時間内で動作するかを検証する負荷テスト。
//
// 使い方:
//   1. ROOM_ID の授業チャットにあらかじめ大量のメッセージを投入しておく
//      (このスクリプトの `k6 run --env SEED=true` で投入することもできる)。
//   2. k6 run load/chat-history-load.js \
//        --env BASE_URL=https://staging.example.com \
//        --env LOAD_TEST_EMAIL=... --env LOAD_TEST_PASSWORD=... \
//        --env ROOM_ID=<opaque room id>
//
// 環境変数:
//   BASE_URL           対象サーバーのベースURL (デフォルト http://localhost:8080)
//   LOAD_TEST_EMAIL/PASSWORD  対象授業に登録済みのアカウント
//   ROOM_ID             検証対象の授業チャットルームの opaque id
//   SEED                "true" の場合、setup() で SEED_COUNT 件のメッセージを事前投入する
//   SEED_COUNT          SEED=true のときに投入する件数 (デフォルト 10000)

import { check, sleep } from 'k6';
import { gql, login, requireRoomID } from './lib.js';

export const options = {
  scenarios: {
    paginate_history: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 10),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    // ページング1リクエストのp95が800ms以内であること(項番77の合格ライン)。
    http_req_duration: ['p(95)<800'],
    checks: ['rate>0.99'],
  },
};

const SEND_MESSAGE = `
  mutation SendMessage($roomID: ID!, $content: String!) {
    sendMessage(roomID: $roomID, content: $content) { ID }
  }
`;

const MESSAGES_PAGE = `
  query Messages($roomID: ID!, $limit: Int, $before: ID) {
    messages(roomID: $roomID, limit: $limit, before: $before) {
      items { ID content createdAt }
      hasMoreBefore
    }
  }
`;

export function setup() {
  const token = login();
  const roomID = requireRoomID();

  if (__ENV.SEED === 'true') {
    const count = Number(__ENV.SEED_COUNT || 10000);
    console.log(`seeding ${count} messages into room ${roomID}...`);
    for (let i = 0; i < count; i++) {
      const res = gql(token, SEND_MESSAGE, { roomID, content: `load-test message #${i}` });
      if (res.status !== 200) {
        throw new Error(`seed message ${i} failed: ${res.status} ${res.body}`);
      }
    }
  }

  return { token, roomID };
}

export default function (data) {
  // 最新ページから開始し、beforeカーソルで過去方向にページングし続ける
  // (管理画面のスクロール/ページング操作を模す)。
  let before = null;
  for (let page = 0; page < 5; page++) {
    const res = gql(data.token, MESSAGES_PAGE, { roomID: data.roomID, limit: 50, before });
    const ok = check(res, {
      'status is 200': (r) => r.status === 200,
      'no graphql errors': (r) => !JSON.parse(r.body).errors,
    });
    if (!ok) break;

    const body = JSON.parse(res.body);
    const page_ = body.data && body.data.messages;
    if (!page_ || page_.items.length === 0 || !page_.hasMoreBefore) break;
    before = page_.items[0].ID;
  }
  sleep(1);
}
