ALTER TABLE blocks
    DROP FOREIGN KEY fk_blocks_account,
    DROP FOREIGN KEY fk_blocks_blocked_account,
    ADD CONSTRAINT blocks_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT blocks_ibfk_2 FOREIGN KEY (blocked_user_id) REFERENCES users(id) ON DELETE CASCADE;
