ALTER TABLE profiles
    ADD COLUMN avatar_media_id BIGINT NULL,
    ADD CONSTRAINT fk_profiles_avatar_media FOREIGN KEY (avatar_media_id) REFERENCES media(id) ON DELETE SET NULL,
    DROP COLUMN avatar_key;
