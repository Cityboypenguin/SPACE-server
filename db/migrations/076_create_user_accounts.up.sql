-- 利用者の「個人情報」を users から切り出す表。
--
-- 退会しても会話（メッセージ・質問・回答・投票）は残し、個人情報は消す。この2つを
-- 同じ行に入れていると、行を残せば個人情報も残り、行を消せば会話まで CASCADE で
-- 消える。寿命の違うものを分けるために表を2つにした:
--
--   users          識別子。会話が参照し続けるので行は消さない（id, status, 日時だけ）
--   user_accounts  個人情報。退会から15日後（管理者による削除は即時）に行ごと消す
--
-- 行ごと消すので、個人情報の列を今後ここへ足しても消し忘れは起きない。一意制約
-- （account_id, email）もこちらに置くので、退会者の行が消えれば同じ値を別の人が
-- 使える（users に仮の値を詰める必要が無い）。
--
-- users への参照は RESTRICT。users の行は消さない前提なので、誤って消そうとしたら
-- 止める。個人情報にぶら下がる表（プロフィール・投稿・フォローなど）は 086〜101 で
-- この表へ付け替え、ここが消えたら一緒に消えるようにする。
CREATE TABLE IF NOT EXISTS user_accounts (
    user_id             BIGINT       NOT NULL,
    account_id          VARCHAR(255) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    email               VARCHAR(255) NOT NULL,
    hashed_password     VARCHAR(255) NOT NULL,
    role                VARCHAR(50)  NOT NULL DEFAULT 'student',
    credentials_version BIGINT       NOT NULL DEFAULT 0,
    last_active_at      BIGINT       NULL,
    updated_at          BIGINT       NOT NULL,
    PRIMARY KEY (user_id),
    UNIQUE KEY uq_user_accounts_account_id (account_id),
    UNIQUE KEY uq_user_accounts_email (email),
    KEY idx_user_accounts_last_active_at (last_active_at),
    CONSTRAINT fk_user_accounts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);
