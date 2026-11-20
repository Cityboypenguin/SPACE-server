CREATE TABLE community_invitations (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    room_id BIGINT NOT NULL,
    sender_id BIGINT NOT NULL,
    recipient_id BIGINT NULL,
    token_digest BINARY(32) NULL,
    state ENUM('pending', 'accepted', 'declined', 'revoked') NOT NULL DEFAULT 'pending',
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    UNIQUE KEY unique_invitation_token (token_digest),
    KEY invitation_recipient (recipient_id, state, expires_at),
    KEY invitation_room (room_id, recipient_id, state, expires_at),
    KEY invitation_sender (sender_id, room_id, created_at),
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    FOREIGN KEY (sender_id) REFERENCES user_accounts(user_id) ON DELETE CASCADE,
    FOREIGN KEY (recipient_id) REFERENCES user_accounts(user_id) ON DELETE CASCADE
);
