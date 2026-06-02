CREATE TABLE IF NOT EXISTS blocks (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    user_id     BIGINT       NOT NULL, /*ブロックする側のユーザーID*/
    blocked_user_id BIGINT   NOT NULL, /*ブロックされる側のユーザーID*/
    created_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_favorite_blocker (user_id, blocked_user_id), /*同じユーザーを複数回ブロックできないようにするための一意制約*/
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE, /*user_idがusersテーブルのidを参照し、ユーザーが削除されたときに関連するブロック登録も削除されるようにする*/
    FOREIGN KEY (blocked_user_id) REFERENCES users(id) ON DELETE CASCADE /*blocked_user_idがusersテーブルのidを参照し、ブロックされる側のユーザーが削除されたときに関連するブロック登録も削除されるようにする*/
);