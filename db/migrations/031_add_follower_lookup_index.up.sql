CREATE INDEX idx_favorite_users_follower_created
ON favorite_users (favorite_user_id, created_at, id);
