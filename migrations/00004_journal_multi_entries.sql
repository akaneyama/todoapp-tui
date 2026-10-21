-- +goose Up
-- Allow multiple journal entries per user per date by dropping the unique constraint on (user_id, date).
ALTER TABLE journal_entries DROP KEY uq_journal_user_date;
ALTER TABLE journal_entries ADD KEY idx_journal_user_date (user_id, date);

-- +goose Down
ALTER TABLE journal_entries DROP KEY idx_journal_user_date;
ALTER TABLE journal_entries ADD UNIQUE KEY uq_journal_user_date (user_id, date);
