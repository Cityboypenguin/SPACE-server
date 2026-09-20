ALTER TABLE timetables
    ADD COLUMN color VARCHAR(20) NOT NULL DEFAULT 'BLUE',
    DROP COLUMN is_profile_visible;
