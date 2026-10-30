-- 退会者を完全削除（user_accounts の行を削除）した後は、その人の account_id / email が
-- 空文字で残るため一意制約を付け直せない。消した個人情報は戻せないので、ここは失敗する。
ALTER TABLE users
    ADD UNIQUE KEY account_id (account_id),
    ADD UNIQUE KEY email (email),
    ADD KEY idx_users_last_active_at (last_active_at);
