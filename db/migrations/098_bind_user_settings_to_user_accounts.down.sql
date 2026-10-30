ALTER TABLE user_settings
    DROP FOREIGN KEY fk_user_settings_account,
    ADD CONSTRAINT fk_user_settings_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
