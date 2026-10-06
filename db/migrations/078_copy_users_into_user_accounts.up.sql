-- 既存の利用者の個人情報を user_accounts へ写す。users の列は 102 で落とす。
INSERT INTO user_accounts
    (user_id, account_id, name, email, hashed_password, role, credentials_version, last_active_at, updated_at)
SELECT id, account_id, name, email, hashed_password, role, credentials_version, last_active_at, updated_at
FROM users;
