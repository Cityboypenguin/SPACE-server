CREATE TABLE IF NOT EXISTS courses (
    id           BIGINT       NOT NULL AUTO_INCREMENT,
    room_id      BIGINT       NOT NULL,
    day_of_week  VARCHAR(10)  NOT NULL,
    period       INT          NOT NULL,
    teacher_name VARCHAR(255) NOT NULL,
    course_name  VARCHAR(255) NOT NULL,
    year         INT          NOT NULL,
    semester     VARCHAR(10)  NOT NULL,
    dedup_key    VARCHAR(255) NOT NULL,
    created_at   BIGINT       NOT NULL,
    updated_at   BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_courses_dedup_key (dedup_key),
    INDEX idx_courses_day_period (day_of_week, period),
    INDEX idx_courses_year_semester (year, semester),
    CONSTRAINT fk_courses_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);
