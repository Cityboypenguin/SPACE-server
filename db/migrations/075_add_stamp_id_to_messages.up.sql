ALTER TABLE messages
    ADD COLUMN stamp_id VARCHAR(32) NULL AFTER content;
