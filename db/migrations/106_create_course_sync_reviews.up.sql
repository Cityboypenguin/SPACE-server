-- シラバス同期で自動では判断しない変化（講義コードの使い回し・振り直しの疑い、
-- 複数コマの対応が付かない）を管理者の確認に回す。
--
-- fingerprint は「同じ状況」を表す値で、再実行で同じ確認が積み増されないようにする。
-- status:
--   PENDING   : 確認待ち。同期はこの授業に触らない
--   SAME      : 管理者が「同じ授業」と判断した。次の同期で反映する
--   DIFFERENT : 管理者が「別の授業」と判断した。次の同期で反映する
--   IGNORED   : 今のまま据え置く。同じ状況では再び確認に回さない
--   APPLIED   : 判断を同期で反映済み
CREATE TABLE IF NOT EXISTS course_sync_reviews (
    id             BIGINT        NOT NULL AUTO_INCREMENT,
    year           INT           NOT NULL,
    kind           VARCHAR(32)   NOT NULL,
    fingerprint    CHAR(64)      NOT NULL,
    status         VARCHAR(16)   NOT NULL,
    message        VARCHAR(1000) NOT NULL,
    existing_json  JSON          NOT NULL,
    proposed_json  JSON          NOT NULL,
    first_run_id   BIGINT        NULL,
    applied_run_id BIGINT        NULL,
    resolved_by    BIGINT        NULL,
    resolved_at    BIGINT        NULL,
    created_at     BIGINT        NOT NULL,
    updated_at     BIGINT        NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_course_sync_reviews_fingerprint (fingerprint),
    INDEX idx_course_sync_reviews_year_status (year, status)
);
