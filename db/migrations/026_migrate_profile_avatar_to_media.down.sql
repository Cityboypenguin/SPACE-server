ALTER TABLE profiles DROP FOREIGN KEY fk_profiles_avatar_media, DROP COLUMN avatar_media_id, ADD COLUMN avatar_key VARCHAR(255) DEFAULT NULL;
