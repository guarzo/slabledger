CREATE TABLE IF NOT EXISTS card_request_submissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cert_number TEXT NOT NULL,
    grader TEXT NOT NULL DEFAULT 'PSA',
    card_name TEXT NOT NULL DEFAULT '',
    set_name TEXT NOT NULL DEFAULT '',
    card_number TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL DEFAULT '',
    front_image_url TEXT NOT NULL DEFAULT '',
    variant TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    cardhedger_request_id TEXT NOT NULL DEFAULT '',
    submitted_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(grader, cert_number)
);
