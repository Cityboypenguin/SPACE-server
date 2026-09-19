# SPACE設計リファクタのリリース注意事項

## 必須migration

`070_create_user_activity_hours.up.sql` と `071_create_user_activity_archives.up.sql` は
この変更に含まれる必須migrationである。
アプリケーションは `user_activity_hours` へ書き込むため、サーバーを更新する前に適用すること。

`user_activity_hours` はMySQLに400日間保持する。それより古い完了済みのJST月は月単位の
`CSV.gz` に変換し、アップロード後に保存先のサイズとSHA-256を読み戻して照合し、
削除対象件数も一致した場合だけ元の行を削除する。失敗・ロック競合時は5分後に再試行する。
アーカイブは3年間保持する。
ジョブはMySQL advisory lockで1インスタンスだけが実行し、JSTの月境界を
MySQLの接続timezoneに依存しない文字列境界として扱う。

CSVのユーザー識別子は生の `user_id` ではなく、`ACTIVITY_ARCHIVE_HMAC_KEY`
によるHMAC-SHA256値である。月跨ぎの集計に使える一方、鍵を保持する限り
退会者を含む同一人物の行を照合できるため、匿名データとは扱わない。
退会時はMySQL内の活動日・時間帯の行をトランザクション内で削除し、遅延記録も
新規作成しない。既存の非公開CSVは個人単位で書き換えず、保管期限（アーカイブ作成から
3年）まで保持し、その後ジョブで削除する。退会時の全履歴消去を保証する設計ではない。
この値には32byte以上の専用秘密値を設定し、
保持期間中は回転させないこと。本番では未設定・短すぎる場合はサーバーが起動しない。
ローカル開発では未設定時のみ開発用の固定鍵を使う（本番では拒否する）。
EC2デプロイ前にSecureString `/space/activity-archive-hmac-key` をSSM Parameter Storeに登録すること。

アーカイブ先は匿名公開されていない専用バケット／コンテナを用意し、MinIOでは
`MINIO_PRIVATE_BUCKET`、Azureでは `AZURE_STORAGE_PRIVATE_CONTAINER_NAME` を設定すること。
未設定時は既存名に `-private` を付けた名前を使用する。
EC2構成では非公開S3 bucket `space-activity-archives` を先に作成し、実行ロールに
PutObject/GetObject/DeleteObjectを許可すること。ローカルComposeは非公開bucketを自動作成する。

## GraphQLの破壊的変更

公開 `User` 型からの `email` 削除は、情報境界を修正するための意図的な破壊的変更である。
`User.email` を選択する既存外部クライアントはGraphQL validationで失敗するため、リリース前に
`me` や管理者専用queryが返す `UserAccount.email` へ移行する必要がある。

deprecatedで残したcommunity/room mutationの互換性とは分けて告知すること。
