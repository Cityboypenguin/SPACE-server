CREATE TABLE IF NOT EXISTS message_reactions (
    id         BIGINT NOT NULL AUTO_INCREMENT,
    message_id BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    emoji      VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_message_reactions_message_user_emoji (message_id, user_id, emoji),
    INDEX idx_message_reactions_user (user_id),
    CONSTRAINT fk_message_reactions_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
    CONSTRAINT fk_message_reactions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
