ALTER TABLE timetables
    DROP FOREIGN KEY fk_timetables_account,
    ADD CONSTRAINT fk_timetables_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
