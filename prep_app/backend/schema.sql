-- Контракт базы, которую можно загрузить позже.
-- Сервер сам создаёт файл и добавляет недостающие колонки,
-- не удаляя уже существующие строки.

CREATE TABLE IF NOT EXISTS prep_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_type TEXT NOT NULL DEFAULT 'flashcard', -- quiz | flashcard | code
    block TEXT NOT NULL DEFAULT 'go',            -- go | db_kafka | algo | system_design
    company TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    options TEXT NOT NULL DEFAULT '',            -- JSON-массив строк, для quiz
    correct_index INTEGER,
    explanation TEXT NOT NULL DEFAULT '',
    answer_skeleton TEXT NOT NULL DEFAULT '',
    checklist TEXT NOT NULL DEFAULT '',          -- JSON-массив тезисов
    starter_code TEXT NOT NULL DEFAULT '',
    reference_code TEXT NOT NULL DEFAULT '',     -- эталон, клиенту не отдаётся
    review_criteria TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    times_asked INTEGER NOT NULL DEFAULT 0,
    times_passed INTEGER NOT NULL DEFAULT 0,
    last_result INTEGER,                         -- 1 зачтено, 0 провал, NULL ещё не было
    closed INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS web_queue (
    question_id INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Отдельный файл hard_concepts.db
CREATE TABLE IF NOT EXISTS hard_concepts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    question_id INTEGER,
    block TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    fail_count INTEGER NOT NULL DEFAULT 1,
    last_failed_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved INTEGER NOT NULL DEFAULT 0
);
