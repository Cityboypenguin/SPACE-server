ALTER TABLE profiles
    DROP FOREIGN KEY fk_profiles_account,
    ADD CONSTRAINT profiles_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
