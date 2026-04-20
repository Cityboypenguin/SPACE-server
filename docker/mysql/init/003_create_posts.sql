CREATE TABLE IF NOT EXISTS posts (
    id              BIGINT       NOT NULL AUTO_INCREMENT,
    user_id         BIGINT       NOT NULL,
    content         TEXT         NOT NULL,
    picture         VARCHAR(255)  NULL,
    movie           VARCHAR(255)  NULL,
    hyperlink       VARCHAR(255)  NULL,
    favorite_count  INT          NOT NULL DEFAULT 0,
    created_at      BIGINT       NOT NULL,
    updated_at      BIGINT       NOT NULL,
    PRIMARY KEY     (id),
    FOREIGN KEY     (user_id) REFERENCES users(id) ON DELETE CASCADE
);