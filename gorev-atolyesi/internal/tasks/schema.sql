-- Migration 001: idempotent initial schema. Further migrations belong in versioned files.
CREATE TABLE IF NOT EXISTS tasks (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 title TEXT NOT NULL CHECK(length(title) BETWEEN 3 AND 120),
 description TEXT NOT NULL DEFAULT '' CHECK(length(description)<=2000),
 category TEXT NOT NULL CHECK(category IN ('Go','Kubernetes','chromedp','Genel')),
 priority TEXT NOT NULL CHECK(priority IN ('low','medium','high')),
 status TEXT NOT NULL CHECK(status IN ('todo','doing','done')),
 due_date TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
PRAGMA user_version=1;
