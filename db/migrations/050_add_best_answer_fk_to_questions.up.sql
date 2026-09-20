ALTER TABLE questions
    ADD CONSTRAINT fk_questions_best_answer FOREIGN KEY (best_answer_id) REFERENCES answers(id) ON DELETE SET NULL;
