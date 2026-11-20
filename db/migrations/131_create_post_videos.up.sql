-- 投稿に付いた動画。1つの投稿に動画は1本まで（post_id が主キー）。
-- 画像の post_media と分けてあるのは、既存の Post.media に動画を混ぜないため
-- （動画を知らない古いクライアントが <img> で描こうとする）。
CREATE TABLE post_videos (
    post_id  BIGINT NOT NULL,
    media_id BIGINT NOT NULL,
    PRIMARY KEY (post_id),
    UNIQUE KEY uk_post_videos_media (media_id),
    CONSTRAINT fk_post_videos_post  FOREIGN KEY (post_id)  REFERENCES posts(id) ON DELETE CASCADE,
    CONSTRAINT fk_post_videos_media FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);
