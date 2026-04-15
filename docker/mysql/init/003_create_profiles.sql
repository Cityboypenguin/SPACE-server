CREATE TABLE IF NOT EXISTS profiles (
    user_id VARCHAR(255) PRIMARY KEY, -- ユーザーモデルのuserIDとリンクさせます
    username VARCHAR(255) NOT NULL,   -- ユーザー名
    bio TEXT,                         -- 自己紹介文（長い文章が入るためTEXT型）
    grade VARCHAR(50),                -- 学年
    image VARCHAR(255),               -- 画像（画像のURLを保存するためVARCHAR型）
    created_at BIGINT NOT NULL,       -- 作成日時
    updated_at BIGINT NOT NULL        -- 更新日時
);