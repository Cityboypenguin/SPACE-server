-- 退会の段階を users に持たせる。
--
-- status は active / frozen に加えて次の2つを取る:
--   deactivated  退会手続き中（15日の猶予）。ログインすれば取り消せる。
--                deactivated_at が猶予の起点
--   deleted      完全削除済み。user_accounts の行はもう無い。deleted_at がその時刻
--
-- 索引は (status, deactivated_at)。日次の削除処理が「猶予の切れた退会手続き中」を
-- 探すのと、表示系が「退会手続き中の人を除く」のに使う。status 単独の索引はこれの
-- 先頭列で足りるので落とす。
ALTER TABLE users
    ADD COLUMN deactivated_at BIGINT NULL AFTER status,
    ADD COLUMN deleted_at     BIGINT NULL AFTER deactivated_at,
    ADD INDEX idx_users_status_deactivated_at (status, deactivated_at),
    DROP INDEX idx_users_status;
