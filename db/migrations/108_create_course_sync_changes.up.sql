-- シラバス同期で変わった（ドライランなら変わる予定の）授業1件ごとの記録。
-- course_id に外部キーは張らない。授業が後で削除されても、何が起きたかの記録は残す。
--
-- unregistered_count はコマが変わった結果、移った先のコマに別の授業を登録していたため
-- 時間割からこの授業を外した人数（時間割は1コマ1授業。元からそのコマにあった授業を残す）。
CREATE TABLE IF NOT EXISTS course_sync_changes (
    id                  BIGINT        NOT NULL AUTO_INCREMENT,
    run_id              BIGINT        NOT NULL,
    kind                VARCHAR(16)   NOT NULL,
    course_id           BIGINT        NULL,
    review_id           BIGINT        NULL,
    course_name         VARCHAR(255)  NOT NULL,
    teacher_name        VARCHAR(255)  NOT NULL,
    detail              VARCHAR(1000) NOT NULL,
    before_json         JSON          NULL,
    after_json          JSON          NULL,
    registered_count    INT           NOT NULL,
    unregistered_count  INT           NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_course_sync_changes_run (run_id, kind, id),
    CONSTRAINT fk_course_sync_changes_run FOREIGN KEY (run_id) REFERENCES course_sync_runs(id) ON DELETE CASCADE
);
