ALTER TABLE messages
    DROP INDEX idx_messages_deleted_at,
    DROP COLUMN deleted_by,
    DROP COLUMN deleted_at;
