-- +goose Up
-- Add title to journal entries and update fulltext search to include title.
ALTER TABLE journal_entries ADD COLUMN title VARCHAR(255) NOT NULL DEFAULT '' AFTER date;
ALTER TABLE journal_entries DROP KEY ft_journal_body;
ALTER TABLE journal_entries ADD FULLTEXT KEY ft_journal_content (title, body);

-- Add description to todos for extended task notes.
ALTER TABLE todos ADD COLUMN description TEXT NULL AFTER title;

-- +goose Down
ALTER TABLE todos DROP COLUMN description;
ALTER TABLE journal_entries DROP KEY ft_journal_content;
ALTER TABLE journal_entries ADD FULLTEXT KEY ft_journal_body (body);
ALTER TABLE journal_entries DROP COLUMN title;
