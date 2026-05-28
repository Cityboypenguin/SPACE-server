CREATE TABLE IF NOT EXISTS media (
    id               BIGINT       NOT NULL AUTO_INCREMENT,
    uploader_user_id BIGINT       NOT NULL,
    storage_key      VARCHAR(255) NOT NULL,
    content_type     VARCHAR(100) NOT NULL,
    created_at       BIGINT       NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (uploader_user_id) REFERENCES users(id) ON DELETE CASCADE
);
