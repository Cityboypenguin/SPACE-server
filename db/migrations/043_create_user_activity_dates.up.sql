CREATE TABLE user_activity_dates (
  user_id      BIGINT NOT NULL,
  activity_date DATE   NOT NULL,
  PRIMARY KEY (user_id, activity_date),
  INDEX idx_activity_date (activity_date)
) ENGINE=InnoDB;
