ALTER TABLE messages
    DROP INDEX idx_messages_created_at,
    DROP INDEX idx_messages_room_created;
