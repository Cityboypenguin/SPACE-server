CREATE TABLE IF NOT EXISTS timetables (
    id                 BIGINT  NOT NULL AUTO_INCREMENT,
    user_id            BIGINT  NOT NULL,
    course_id          BIGINT  NOT NULL,
    is_profile_visible BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         BIGINT  NOT NULL,
    updated_at         BIGINT  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_timetables_user_course (user_id, course_id),
    INDEX idx_timetables_user (user_id),
    CONSTRAINT fk_timetables_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_timetables_course FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
);
