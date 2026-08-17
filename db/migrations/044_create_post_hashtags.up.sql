CREATE TABLE post_hashtags (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    post_id    BIGINT       NOT NULL,
    tag        VARCHAR(255) NOT NULL,
    created_at BIGINT       NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_post_hashtags_tag (tag),
    INDEX idx_post_hashtags_post (post_id),
    CONSTRAINT fk_post_hashtags_post FOREIGN KEY (post_id)
        REFERENCES posts(id) ON DELETE CASCADE
);
