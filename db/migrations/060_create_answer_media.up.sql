CREATE TABLE IF NOT EXISTS answer_media (
    id        BIGINT NOT NULL AUTO_INCREMENT,
    answer_id BIGINT NOT NULL,
    media_id  BIGINT NOT NULL,
    position  INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    INDEX idx_answer_media_answer (answer_id),
    FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);
