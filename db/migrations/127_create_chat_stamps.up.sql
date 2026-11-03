CREATE TABLE chat_stamps (
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 label VARCHAR(50) NOT NULL,
 image_key VARCHAR(512) NOT NULL DEFAULT '',
 display_size VARCHAR(10) NOT NULL DEFAULT 'standard',
 status VARCHAR(10) NOT NULL DEFAULT 'draft',
 allow_message BOOLEAN NOT NULL DEFAULT TRUE,
 allow_reaction BOOLEAN NOT NULL DEFAULT TRUE,
 message_order INT NOT NULL DEFAULT 0,
 reaction_order INT NOT NULL DEFAULT 0,
 version BIGINT NOT NULL DEFAULT 1,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 created_by BIGINT NOT NULL,
 updated_by BIGINT NOT NULL,
 INDEX idx_chat_stamps_status (status)
);
