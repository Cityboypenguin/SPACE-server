-- users の一意制約と last_active_at の索引を外す（列そのものは 102 で落とす）。
--
-- 一意制約は 076 の user_accounts へ移った。ここで先に外しておくのは down のため:
-- 逆順に戻すとき、102 の down が空の列を足し、078 の down が値を書き戻してから
-- この down が一意制約を付け直す。順番を逆にすると、空文字が並んだ列に一意制約を
-- 付けることになって失敗する。
ALTER TABLE users
    DROP INDEX account_id,
    DROP INDEX email,
    DROP INDEX idx_users_last_active_at;
