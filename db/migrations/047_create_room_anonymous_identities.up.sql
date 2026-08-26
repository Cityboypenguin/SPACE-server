CREATE TABLE IF NOT EXISTS room_anonymous_identities (
    id         BIGINT      NOT NULL AUTO_INCREMENT,
    room_id    BIGINT      NOT NULL,
    user_id    BIGINT      NOT NULL,
    label      VARCHAR(50) NOT NULL,
    created_at BIGINT      NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_room_anon_room_user (room_id, user_id),
    CONSTRAINT fk_room_anon_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_room_anon_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
