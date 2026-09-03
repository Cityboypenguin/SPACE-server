CREATE TABLE IF NOT EXISTS question_media (
    id          BIGINT NOT NULL AUTO_INCREMENT,
    question_id BIGINT NOT NULL,
    media_id    BIGINT NOT NULL,
    position    INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    INDEX idx_question_media_question (question_id),
    FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);
