-- メッセージ（DM・コミュニティ・授業内チャット）に付いた動画。1件に1本まで。
CREATE TABLE message_videos (
    message_id BIGINT NOT NULL,
    media_id   BIGINT NOT NULL,
    PRIMARY KEY (message_id),
    UNIQUE KEY uk_message_videos_media (media_id),
    CONSTRAINT fk_message_videos_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
    CONSTRAINT fk_message_videos_media   FOREIGN KEY (media_id)   REFERENCES media(id)    ON DELETE CASCADE
);
