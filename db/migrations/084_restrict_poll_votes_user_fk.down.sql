ALTER TABLE poll_votes
    DROP FOREIGN KEY fk_poll_votes_voter,
    ADD CONSTRAINT fk_poll_votes_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
