CREATE TABLE IF NOT EXISTS notifications (
    id          BIGINT NOT NULL AUTO_INCREMENT,
    user_id     BIGINT NOT NULL,
    type        VARCHAR(64) NOT NULL,
    actor_id    BIGINT,
    target_type VARCHAR(64),
    target_id   BIGINT,
    message     TEXT NOT NULL,
    is_read     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  BIGINT NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_notifications_user_read (user_id, is_read),
    INDEX idx_notifications_user_created (user_id, created_at)
);
