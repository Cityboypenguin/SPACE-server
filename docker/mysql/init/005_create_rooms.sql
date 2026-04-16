CREATE TABLE IF NOT EXISTS rooms (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    name        VARCHAR(255),
    type        VARCHAR(50)  NOT NULL,
    description TEXT         NOT NULL,
    created_at  BIGINT       NOT NULL,
    updated_at  BIGINT       NOT NULL,
    PRIMARY KEY (id)
);