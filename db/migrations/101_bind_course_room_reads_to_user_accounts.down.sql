ALTER TABLE course_room_reads
    DROP FOREIGN KEY fk_course_room_reads_account,
    ADD CONSTRAINT fk_course_room_reads_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
