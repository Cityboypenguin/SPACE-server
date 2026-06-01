CREATE TABLE IF NOT EXISTS message_media (
    id         BIGINT NOT NULL AUTO_INCREMENT,
    message_id BIGINT NOT NULL,
    media_id   BIGINT NOT NULL,
    position   INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id)   REFERENCES media(id)    ON DELETE CASCADE
);
