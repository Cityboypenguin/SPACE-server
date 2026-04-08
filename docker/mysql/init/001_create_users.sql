CREATE TABLE IF NOT EXISTS users (
    id              BIGINT       NOT NULL AUTO_INCREMENT,
    user_id         VARCHAR(255) NOT NULL UNIQUE,
    name            VARCHAR(255) NOT NULL,
    email           VARCHAR(255) NOT NULL UNIQUE,
    hashed_password VARCHAR(255) NOT NULL,
    role            VARCHAR(50)  NOT NULL DEFAULT 'user',
    status          VARCHAR(50)  NOT NULL DEFAULT 'active',
    created_at      BIGINT       NOT NULL,
    updated_at      BIGINT       NOT NULL,
    PRIMARY KEY (id)
);
