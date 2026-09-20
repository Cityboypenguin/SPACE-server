ALTER TABLE messages
    ADD COLUMN deleted_at BIGINT NULL AFTER updated_at,
    ADD COLUMN deleted_by BIGINT NULL AFTER deleted_at,
    ADD INDEX idx_messages_deleted_at (deleted_at);
