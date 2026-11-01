-- 回答への追記（ANS-07）。追記できるのは回答者本人だけ。質問への追記と同じ考え方。
CREATE TABLE IF NOT EXISTS answer_addenda (
    id         BIGINT NOT NULL AUTO_INCREMENT,
    answer_id  BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    body       TEXT   NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_answer_addenda_answer (answer_id, id),
    CONSTRAINT fk_answer_addenda_answer FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE,
    CONSTRAINT fk_answer_addenda_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)
