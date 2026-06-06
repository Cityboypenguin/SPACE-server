CREATE TABLE IF NOT EXISTS terms_of_service (
    id             BIGINT NOT NULL AUTO_INCREMENT,
    version        VARCHAR(50) NOT NULL,
    object_key     VARCHAR(500) NOT NULL,
    effective_date BIGINT NOT NULL,
    created_at     BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_terms_version (version),
    INDEX idx_terms_effective (effective_date)
);
