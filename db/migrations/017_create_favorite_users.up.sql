CREATE TABLE IF NOT EXISTS favorite_users (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    user_id     BIGINT       NOT NULL,
    favorite_user_id BIGINT   NOT NULL,
    created_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_favorite_user (user_id, favorite_user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (favorite_user_id) REFERENCES users(id) ON DELETE CASCADE
);