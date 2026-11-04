ALTER TABLE answer_likes
    DROP FOREIGN KEY fk_answer_likes_liker,
    ADD CONSTRAINT fk_answer_likes_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
