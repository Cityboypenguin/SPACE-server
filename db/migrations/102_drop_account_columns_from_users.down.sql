-- 列だけを空で戻す。値は 078 の down が user_accounts から書き戻し、一意制約は
-- 077 の down が付け直す。
ALTER TABLE users
    ADD COLUMN account_id          VARCHAR(255) NOT NULL DEFAULT '' AFTER id,
    ADD COLUMN name                VARCHAR(255) NOT NULL DEFAULT '' AFTER account_id,
    ADD COLUMN email               VARCHAR(255) NOT NULL DEFAULT '' AFTER name,
    ADD COLUMN hashed_password     VARCHAR(255) NOT NULL DEFAULT '' AFTER email,
    ADD COLUMN role                VARCHAR(50)  NOT NULL DEFAULT 'student' AFTER hashed_password,
    ADD COLUMN updated_at          BIGINT       NOT NULL DEFAULT 0 AFTER created_at,
    ADD COLUMN last_active_at      BIGINT       NULL,
    ADD COLUMN credentials_version BIGINT       NOT NULL DEFAULT 0;
