CREATE TABLE IF NOT EXISTS room_users (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    room_id     BIGINT       NOT NULL,
    user_id     BIGINT       NOT NULL,
    created_at  BIGINT       NOT NULL,
    updated_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);