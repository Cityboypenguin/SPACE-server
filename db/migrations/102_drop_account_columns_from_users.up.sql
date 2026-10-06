-- 個人情報の列を users から落とす。値は 078 で user_accounts へ写してある。
--
-- これで users に残るのは id, status, created_at, deactivated_at, deleted_at だけになる。
-- 退会者の行を残しても、名前・メールアドレス・パスワードは users のどこにも無い。
ALTER TABLE users
    DROP COLUMN account_id,
    DROP COLUMN name,
    DROP COLUMN email,
    DROP COLUMN hashed_password,
    DROP COLUMN role,
    DROP COLUMN credentials_version,
    DROP COLUMN last_active_at,
    DROP COLUMN updated_at;
