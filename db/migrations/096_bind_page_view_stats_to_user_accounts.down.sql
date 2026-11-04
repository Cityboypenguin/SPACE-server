ALTER TABLE page_view_stats
    DROP FOREIGN KEY fk_page_view_stats_account,
    ADD CONSTRAINT page_view_stats_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
