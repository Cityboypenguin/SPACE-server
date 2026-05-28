CREATE TABLE IF NOT EXISTS post_media (
    id       BIGINT NOT NULL AUTO_INCREMENT,
    post_id  BIGINT NOT NULL,
    media_id BIGINT NOT NULL,
    position INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    FOREIGN KEY (post_id)  REFERENCES posts(id)  ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id)  ON DELETE CASCADE
);
