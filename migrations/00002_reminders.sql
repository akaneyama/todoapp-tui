-- +goose Up
-- Add reminded_at to track when reminders have been sent to avoid duplicate alerts.
ALTER TABLE todos ADD COLUMN reminded_at DATETIME(6) NULL AFTER remind_at;
ALTER TABLE todos ADD KEY idx_todos_remind_pending (remind_at, reminded_at, done, deleted_at);

-- +goose Down
ALTER TABLE todos DROP KEY idx_todos_remind_pending;
ALTER TABLE todos DROP COLUMN reminded_at;
