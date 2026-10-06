ALTER TABLE media
    DROP FOREIGN KEY fk_media_uploader_account,
    ADD CONSTRAINT media_ibfk_1 FOREIGN KEY (uploader_user_id) REFERENCES users(id) ON DELETE CASCADE;
