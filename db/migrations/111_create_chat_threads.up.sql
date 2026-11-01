CREATE TABLE chat_threads (
    room_id BIGINT NOT NULL PRIMARY KEY,
    created_by BIGINT NULL,
    closed_at BIGINT NULL,
    CONSTRAINT fk_chat_threads_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_chat_threads_creator FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);
