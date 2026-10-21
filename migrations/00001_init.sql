-- +goose Up
-- Initial schema. All timestamps are stored in UTC (the server forces the
-- session time_zone to '+00:00'). DATETIME(6) gives microsecond precision,
-- which keeps `since`-based incremental sync from missing rows that were
-- updated within the same second.

CREATE TABLE users (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    username      VARCHAR(32)     NOT NULL,
    password_hash VARCHAR(255)    NOT NULL,
    timezone      VARCHAR(64)     NOT NULL DEFAULT 'UTC',
    created_at    DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at    DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE sessions (
    token_hash   BINARY(32)      NOT NULL,
    user_id      BIGINT UNSIGNED NOT NULL,
    expires_at   DATETIME(6)     NOT NULL,
    created_at   DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    last_used_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (token_hash),
    KEY idx_sessions_user (user_id),
    KEY idx_sessions_expires (expires_at),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE journal_entries (
    id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    date       DATE            NOT NULL,
    body       MEDIUMTEXT      NOT NULL,
    tags       VARCHAR(512)    NOT NULL DEFAULT '',
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6)     NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_journal_user_date (user_id, date),
    KEY idx_journal_user_updated (user_id, updated_at),
    FULLTEXT KEY ft_journal_body (body),
    CONSTRAINT fk_journal_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE todos (
    id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    date        DATE            NOT NULL,
    title       VARCHAR(500)    NOT NULL,
    done        BOOLEAN         NOT NULL DEFAULT FALSE,
    priority    TINYINT UNSIGNED NOT NULL DEFAULT 2,
    remind_at   DATETIME(6)     NULL,
    repeat_rule ENUM('none','daily','weekly') NOT NULL DEFAULT 'none',
    created_at  DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at  DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at  DATETIME(6)     NULL,
    PRIMARY KEY (id),
    KEY idx_todos_user_date (user_id, date),
    KEY idx_todos_user_updated (user_id, updated_at),
    KEY idx_todos_remind (remind_at),
    CONSTRAINT chk_todos_priority CHECK (priority BETWEEN 1 AND 3),
    CONSTRAINT fk_todos_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE rooms (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(64)     NOT NULL,
    is_dm      BOOLEAN         NOT NULL DEFAULT FALSE,
    -- For DMs: "<minUserID>:<maxUserID>", guarantees one DM room per pair.
    -- NULL for regular rooms (UNIQUE allows multiple NULLs).
    dm_key     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_rooms_dm_key (dm_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE room_members (
    room_id   BIGINT UNSIGNED NOT NULL,
    user_id   BIGINT UNSIGNED NOT NULL,
    joined_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (room_id, user_id),
    KEY idx_room_members_user (user_id),
    CONSTRAINT fk_room_members_room FOREIGN KEY (room_id) REFERENCES rooms (id) ON DELETE CASCADE,
    CONSTRAINT fk_room_members_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE messages (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    room_id    BIGINT UNSIGNED NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    body       TEXT            NOT NULL,
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    -- Cursor pagination: WHERE room_id = ? AND id < ? ORDER BY id DESC
    KEY idx_messages_room_id (room_id, id),
    KEY idx_messages_user (user_id),
    CONSTRAINT fk_messages_room FOREIGN KEY (room_id) REFERENCES rooms (id) ON DELETE CASCADE,
    CONSTRAINT fk_messages_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE read_state (
    user_id              BIGINT UNSIGNED NOT NULL,
    room_id              BIGINT UNSIGNED NOT NULL,
    last_read_message_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    updated_at           DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id, room_id),
    KEY idx_read_state_room (room_id),
    CONSTRAINT fk_read_state_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_read_state_room FOREIGN KEY (room_id) REFERENCES rooms (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS read_state;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS todos;
DROP TABLE IF EXISTS journal_entries;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
