CREATE TABLE community_invitation_uses (
    invitation_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (invitation_id, user_id),
    KEY invitation_use_user (user_id),
    FOREIGN KEY (invitation_id) REFERENCES community_invitations(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES user_accounts(user_id) ON DELETE CASCADE
);
