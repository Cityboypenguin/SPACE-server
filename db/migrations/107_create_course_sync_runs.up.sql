-- シラバス同期の1回ぶんの実行結果（成功した実行だけ。失敗は取り込み状態の errorMessage に出る）。
-- dry_run が真の行は「実行したら何が変わるか」の見積もりで、DB の授業には何も書いていない。
CREATE TABLE IF NOT EXISTS course_sync_runs (
    id                         BIGINT       NOT NULL AUTO_INCREMENT,
    year                       INT          NOT NULL,
    dry_run                    BOOLEAN      NOT NULL,
    triggered_by               BIGINT       NULL,
    site_total                 INT          NOT NULL,
    listed_rows                INT          NOT NULL,
    unidentified_rows          INT          NOT NULL,
    created_count              INT          NOT NULL,
    updated_count              INT          NOT NULL,
    discontinued_count         INT          NOT NULL,
    restored_count             INT          NOT NULL,
    review_count               INT          NOT NULL,
    unchanged_count            INT          NOT NULL,
    unregistered_count         INT          NOT NULL,
    discontinue_skipped_reason VARCHAR(255) NULL,
    started_at                 BIGINT       NOT NULL,
    finished_at                BIGINT       NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_course_sync_runs_year (year, id)
);
