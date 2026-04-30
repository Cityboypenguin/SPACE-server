DROP TABLE IF EXISTS profiles;

CREATE TABLE profiles (
    user_id BIGINT PRIMARY KEY,   
    bio TEXT,
    grade INT,
    image VARCHAR(255),
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE 
);