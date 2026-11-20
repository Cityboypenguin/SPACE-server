CREATE TABLE community_welcome_replies (
    notice_message_id BIGINT NOT NULL,
    sender_id BIGINT NOT NULL,
    PRIMARY KEY (notice_message_id, sender_id),
    FOREIGN KEY (notice_message_id) REFERENCES messages(id) ON DELETE CASCADE,
    FOREIGN KEY (sender_id) REFERENCES user_accounts(user_id) ON DELETE CASCADE
);
