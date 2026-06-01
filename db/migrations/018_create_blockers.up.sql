CREATE TABLE IF NOT EXISTS blocks (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    user_id     BIGINT       NOT NULL,
    blocked_user_id BIGINT   NOT NULL,
    created_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_favorite_blocker (user_id, blocked_user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_user_id) REFERENCES users(id) ON DELETE CASCADE
);