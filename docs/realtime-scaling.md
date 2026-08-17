# リアルタイム配信の水平スケール（将来の複数インスタンス化に向けて）

## 現状（2026-07 時点）

本番は **単一 app インスタンス**（[compose.ec2.yaml](../compose.ec2.yaml)）で運用している。
そのためリアルタイム系はプロセス内メモリ実装で**正しく動作する**：

- [internal/pubsub/pubsub.go](../internal/pubsub/pubsub.go) — GraphQL WebSocket サブスクリプション（DM/未読/既読）。
- [internal/sse/broker.go](../internal/sse/broker.go) — 通知の SSE 配信（履歴リプレイ付き）。

この文書は「app を 2 台以上に増やしたとき」に必要な変更をまとめた**設計メモ**である。
単一インスタンスのうちは実装しない（メリットがなくリアルタイム経路のリスクだけ増えるため）。

## 問題

インスタンス A で発生したイベント（例: ユーザー X が投稿にいいね）を、
別インスタンス B に SSE/WS 接続しているユーザー Y へ届ける手段がない。
接続とイベント発生元が別プロセスに分かれると配信が欠落する。

## 方針: Redis Pub/Sub でファンアウト

Redis は既に導入済み（トークン失効・OTP 等）。これを配信バスとして使う。

1. **発行**: `PublishToUser` / `Broadcast` / `PubSub.Publish` は、ローカル配信の代わりに
   Redis チャンネルへ publish する（ペイロードは JSON）。
2. **購読**: 各インスタンスが起動時に Redis チャンネルを subscribe し、
   受信したイベントを**自分のローカル接続にだけ**配信する（既存の broker/pubsub のローカル配信処理を再利用）。
3. これで発生元に関係なく全インスタンスの接続へ届く。単一インスタンスでも
   「publish → 自分が受信 → ローカル配信」で従来と同じ挙動になる。

### 実装上の注意

- **SSE (broker.go)**: ペイロードが既に `map[string]any` なので JSON 化は容易。
  イベント ID / 履歴はインスタンスローカルのままでよい（SSE 接続はスティッキーにするのが前提。
  ロードバランサで同一ユーザーの SSE を同一インスタンスへ固定する）。リプレイは DB の通知一覧が最終的な真実。
- **PubSub (pubsub.go)**: ここが難所。現在は Go の構造体ポインタ（`*gqlmodel.Message` 等）を
  チャンネルにそのまま流している。Redis を挟むにはトピックごとに **JSON シリアライズ/デシリアライズ**が必要。
  トピック名から具体型を解決するレジストリ（`map[topicPattern]func([]byte) any`）を用意し、
  購読側で正しい型に復元してからローカル配信する。
- **接続数メトリクス** (`metrics.Global.IncSSEConnections` 等) はローカル接続数のまま各インスタンスで計測し、
  全体値が必要なら集計側で合算する。

### 代替案

将来 RDS/ElastiCache などマネージド構成へ移すなら、
GraphQL サブスクリプションを Redis Streams や専用の pub/sub SaaS に載せる選択肢もある。
まずは上記の Redis Pub/Sub ファンアウトが最小変更で確実。
