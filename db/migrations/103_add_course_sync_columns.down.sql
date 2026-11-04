ALTER TABLE courses
    DROP INDEX idx_courses_year_source_ref,
    DROP COLUMN discontinued_at,
    DROP COLUMN source_name,
    DROP COLUMN source_ref,
    DROP COLUMN source;
