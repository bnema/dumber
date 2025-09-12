-- Initial database schema for dumber
-- History tracking for visited URLs

CREATE TABLE IF NOT EXISTS history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    visit_count INTEGER DEFAULT 1,
    last_visited DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_history_url ON history(url);
CREATE INDEX IF NOT EXISTS idx_history_last_visited ON history(last_visited);

-- URL shortcuts configuration
CREATE TABLE IF NOT EXISTS shortcuts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    shortcut TEXT NOT NULL UNIQUE,
    url_template TEXT NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO shortcuts (shortcut, url_template, description) VALUES
    ('g', 'https://www.google.com/search?q=%s', 'Google search'),
    ('gh', 'https://github.com/search?q=%s', 'GitHub search'),
    ('yt', 'https://www.youtube.com/results?search_query=%s', 'YouTube search'),
    ('w', 'https://en.wikipedia.org/wiki/%s', 'Wikipedia search');