CREATE TABLE user_settings (
  user_id    BIGINT       NOT NULL,
  `key`      VARCHAR(255) NOT NULL,
  `value`    VARCHAR(255) NOT NULL,
  updated_at INT          NOT NULL,
  PRIMARY KEY (user_id, `key`),
  CONSTRAINT fk_user_settings_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB;
