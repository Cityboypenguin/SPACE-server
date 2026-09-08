// 項番78: 50〜100人規模の同時接続時に、リアルタイム配信の遅延・欠落が
// 許容範囲内かを検証する負荷テスト。
//
// 各VUは GraphQL over WebSocket (graphql-transport-ws) で messageAdded
// subscription を購読した後、自分自身宛にタイムスタンプ入りのメッセージを
// HTTP 経由で送信し、そのメッセージを自分の購読で受信するまでの時間を
// 配信遅延の指標として計測する。同一ルームに多数のVUが同時購読することで、
// ブロードキャスト配信の負荷状況を再現する。
//
// 使い方:
//   k6 run load/chat-realtime-load.js \
//     --env BASE_URL=https://staging.example.com \
//     --env LOAD_TEST_EMAIL=... --env LOAD_TEST_PASSWORD=... \
//     --env ROOM_ID=<opaque room id> --env VUS=100

import ws from 'k6/ws';
import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import { gql, login, requireRoomID, WS_URL } from './lib.js';

const deliveryLatency = new Trend('message_delivery_latency_ms', true);
const deliveryRate = new Rate('message_delivered');

export const options = {
  scenarios: {
    concurrent_subscribers: {
      executor: 'per-vu-iterations',
      vus: Number(__ENV.VUS || 50),
      iterations: 1,
      maxDuration: '2m',
    },
  },
  thresholds: {
    // 配信漏れが1%未満、p95遅延2秒未満であること(項番78の合格ライン。
    // 環境に応じて調整する)。
    message_delivered: ['rate>0.99'],
    message_delivery_latency_ms: ['p(95)<2000'],
  },
};

const SEND_MESSAGE = `
  mutation SendMessage($roomID: ID!, $content: String!) {
    sendMessage(roomID: $roomID, content: $content) { ID }
  }
`;

export function setup() {
  const token = login();
  const roomID = requireRoomID();
  return { token, roomID };
}

export default function (data) {
  const wsURL = `${WS_URL}/query`;
  const marker = `load-test-vu-${__VU}-${Date.now()}`;
  let sentAt = 0;
  let delivered = false;

  const res = ws.connect(wsURL, { headers: {}, subprotocol: 'graphql-transport-ws' }, function (socket) {
    socket.on('open', function () {
      socket.send(JSON.stringify({
        type: 'connection_init',
        payload: { Authorization: `Bearer ${data.token}` },
      }));
    });

    socket.on('message', function (raw) {
      const msg = JSON.parse(raw);

      if (msg.type === 'connection_ack') {
        socket.send(JSON.stringify({
          id: '1',
          type: 'subscribe',
          payload: {
            query: `subscription MessageAdded($roomID: ID!) { messageAdded(roomID: $roomID) { content } }`,
            variables: { roomID: data.roomID },
          },
        }));

        // 購読確立後、自分宛のマーカー入りメッセージを送信して配信を測る。
        sentAt = Date.now();
        gql(data.token, SEND_MESSAGE, { roomID: data.roomID, content: marker });
        return;
      }

      if (msg.type === 'next' && msg.payload && msg.payload.data && msg.payload.data.messageAdded) {
        if (msg.payload.data.messageAdded.content === marker) {
          delivered = true;
          deliveryLatency.add(Date.now() - sentAt);
          socket.close();
        }
      }
    });

    socket.setTimeout(function () {
      socket.close();
    }, 15000);
  });

  check(res, { 'ws connected': (r) => r && r.status === 101 });
  deliveryRate.add(delivered);
}
