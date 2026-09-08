// k6 の負荷テストスクリプト間で共有するヘルパー。
// GraphQL エンドポイントへのログイン・POSTラッパーのみを提供する。

import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
export const WS_URL = __ENV.WS_URL || BASE_URL.replace(/^http/, 'ws');

// GraphQL へ 1 リクエスト送る薄いラッパー。エラー時は throw せず、
// 呼び出し側が check() で判定できるようレスポンスをそのまま返す。
export function gql(token, query, variables) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  return http.post(`${BASE_URL}/query`, JSON.stringify({ query, variables }), { headers });
}

// メールアドレス/パスワードでログインし、アクセストークンを取得する。
// 環境変数 LOAD_TEST_EMAIL / LOAD_TEST_PASSWORD で対象アカウントを指定する。
// 事前にそのアカウントが対象授業(ROOM_ID)に登録済みであること。
export function login() {
  const email = __ENV.LOAD_TEST_EMAIL;
  const password = __ENV.LOAD_TEST_PASSWORD;
  if (!email || !password) {
    throw new Error('LOAD_TEST_EMAIL / LOAD_TEST_PASSWORD must be set (a student registered to the target course room).');
  }

  const query = `
    mutation LoginUser($input: LoginInput!) {
      loginUser(input: $input) { token }
    }
  `;
  const res = gql(null, query, { input: { email, password } });
  check(res, { 'login succeeded': (r) => r.status === 200 && !!JSON.parse(r.body).data?.loginUser?.token });
  return JSON.parse(res.body).data.loginUser.token;
}

export function requireRoomID() {
  const roomID = __ENV.ROOM_ID;
  if (!roomID) {
    throw new Error('ROOM_ID must be set to the opaque room id (graphID) of the course chat room under test.');
  }
  return roomID;
}
