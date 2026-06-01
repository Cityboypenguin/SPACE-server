CREATE TABLE IF NOT EXISTS favorite_users (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    user_id     BIGINT       NOT NULL, /*お気に入り登録する側のユーザーID*/
    favorite_user_id BIGINT   NOT NULL, /*お気に入り登録される側のユーザーID*/
    created_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_favorite_user (user_id, favorite_user_id), /*同じユーザーを複数回お気に入り登録できないようにするための一意制約*/
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE, /*user_idがusersテーブルのidを参照し、ユーザーが削除されたときに関連するお気に入り登録も削除されるようにする*/
    FOREIGN KEY (favorite_user_id) REFERENCES users(id) ON DELETE CASCADE /*favorite_user_idがusersテーブルのidを参照し、お気に入り登録される側のユーザーが削除されたときに関連するお気に入り登録も削除されるようにする*/
);