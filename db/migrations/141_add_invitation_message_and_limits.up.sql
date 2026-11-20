ALTER TABLE community_invitations
    ADD COLUMN message VARCHAR(200) NULL,
    ADD COLUMN use_count INT NOT NULL DEFAULT 0,
    ADD COLUMN responded_at BIGINT NULL;
