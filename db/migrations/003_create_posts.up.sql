CREATE TABLE IF NOT EXISTS posts (
    id              BIGINT       NOT NULL AUTO_INCREMENT,
    content         TEXT         NOT NULL,
    created_at      BIGINT       NOT NULL,
    updated_at      BIGINT       NOT NULL,
    user_id         BIGINT       NOT NULL,
    parent_id       BIGINT       NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id)   REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_id) REFERENCES posts(id) ON DELETE CASCADE
);