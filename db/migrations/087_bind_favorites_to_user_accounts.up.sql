-- favorites は本人の持ち物なので、users から個人情報（user_accounts）へ参照を付け替える。
-- 退会から15日後（管理者による削除は即時）に user_accounts の行が消えると、CASCADE で一緒に消える。
ALTER TABLE favorites
    DROP FOREIGN KEY favorites_ibfk_1,
    ADD CONSTRAINT fk_favorites_account FOREIGN KEY (user_id) REFERENCES user_accounts(user_id) ON DELETE CASCADE;
