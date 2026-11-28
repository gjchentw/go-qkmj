CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    username_lower TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    balance INTEGER NOT NULL DEFAULT 20000,
    games_played INTEGER NOT NULL DEFAULT 0,
    self_draw_wins INTEGER NOT NULL DEFAULT 0,
    discard_wins INTEGER NOT NULL DEFAULT 0,
    deal_ins INTEGER NOT NULL DEFAULT 0,
    draws INTEGER NOT NULL DEFAULT 0,
    abandoned_hands INTEGER NOT NULL DEFAULT 0,
    login_count INTEGER NOT NULL DEFAULT 0,
    last_login_at TEXT,
    last_node_name TEXT NOT NULL DEFAULT '',
    banned_until TEXT,
    banned_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_accounts_rank ON accounts (games_played, balance DESC);

CREATE TABLE IF NOT EXISTS account_roles (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role_name TEXT NOT NULL,
    PRIMARY KEY (account_id, role_name)
);

CREATE INDEX IF NOT EXISTS idx_account_roles_role ON account_roles (role_name);

CREATE TABLE IF NOT EXISTS moderation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_account_id INTEGER NOT NULL REFERENCES accounts(id),
    target_account_id INTEGER NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    expires_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_moderation_logs_target ON moderation_logs (target_account_id, id DESC);

CREATE TABLE IF NOT EXISTS hand_settlements (
    hand_id TEXT PRIMARY KEY,
    node_name TEXT NOT NULL,
    end_reason TEXT NOT NULL,
    entries_json TEXT NOT NULL,
    settled_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hand_settlements_node ON hand_settlements (node_name, settled_at DESC);

CREATE TABLE IF NOT EXISTS user_preferences (
    account_id INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    prefs_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_blocks (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    blocked_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (account_id, blocked_account_id),
    CHECK (account_id != blocked_account_id)
);
