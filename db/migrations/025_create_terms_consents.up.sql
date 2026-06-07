CREATE TABLE IF NOT EXISTS terms_consents (
    id           BIGINT NOT NULL AUTO_INCREMENT,
    user_id      BIGINT NOT NULL,
    terms_id     BIGINT NOT NULL,
    consented_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_consents_user_terms (user_id, terms_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (terms_id) REFERENCES terms_of_service(id) ON DELETE CASCADE
);
