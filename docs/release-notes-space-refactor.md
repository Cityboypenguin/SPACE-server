# SPACE設計リファクタのリリース注意事項

## 必須migration

`070_create_user_activity_hours.up.sql` はこの変更に含まれる必須migrationである。
アプリケーションは `user_activity_hours` へ書き込むため、サーバーを更新する前に適用すること。

同テーブルの保持期間と削除ジョブは未決定である。migration内に記載した行数見積もりを基に、
`user_activity_dates`、`user_session_summaries`、`page_view_stats` と併せて運用方針を決める。

## GraphQLの破壊的変更

公開 `User` 型からの `email` 削除は、情報境界を修正するための意図的な破壊的変更である。
`User.email` を選択する既存外部クライアントはGraphQL validationで失敗するため、リリース前に
`me` や管理者専用queryが返す `UserAccount.email` へ移行する必要がある。

deprecatedで残したcommunity/room mutationの互換性とは分けて告知すること。
