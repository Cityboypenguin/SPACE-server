-- 102 の down で users に戻した（空の）列へ、user_accounts の値を書き戻す。
UPDATE users u
JOIN user_accounts a ON a.user_id = u.id
SET u.account_id          = a.account_id,
    u.name                = a.name,
    u.email               = a.email,
    u.hashed_password     = a.hashed_password,
    u.role                = a.role,
    u.credentials_version = a.credentials_version,
    u.last_active_at      = a.last_active_at,
    u.updated_at          = a.updated_at;
