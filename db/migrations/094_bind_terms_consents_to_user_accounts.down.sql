ALTER TABLE terms_consents
    DROP FOREIGN KEY fk_terms_consents_account,
    ADD CONSTRAINT terms_consents_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
