-- 管理者が利用者の目から外した授業内チャットの画像。
--
-- media の行はこの時点で消す（利用者向けの取得に除外の条件を足さずに済むように）。元に戻すのに
-- 要る値をここへ写し、media_id は元の media.id のまま持つ。外部キーは張らない。投稿者の退会や
-- 投稿の削除で行が消えると、残したファイルを自動削除で見つけられなくなるため。
-- ファイルは非公開の置き場の removed_key にあり、通報のない画像は removed_at から30日で消す。
CREATE TABLE removed_course_chat_images (
    media_id         BIGINT       NOT NULL,
    source           VARCHAR(20)  NOT NULL,
    target_id        BIGINT       NOT NULL,
    position         INT          NOT NULL DEFAULT 0,
    room_id          BIGINT       NOT NULL,
    course_name      VARCHAR(255) NOT NULL,
    uploader_user_id BIGINT       NOT NULL,
    removed_key      VARCHAR(255) NOT NULL,
    original_key     VARCHAR(255) NOT NULL,
    content_type     VARCHAR(100) NOT NULL,
    width            INT          NULL,
    height           INT          NULL,
    created_at       BIGINT       NOT NULL,
    removed_at       BIGINT       NOT NULL,
    removed_by       BIGINT       NOT NULL,
    PRIMARY KEY (media_id),
    INDEX idx_removed_course_chat_images_removed_at (removed_at),
    INDEX idx_removed_course_chat_images_room (room_id, removed_at)
);
