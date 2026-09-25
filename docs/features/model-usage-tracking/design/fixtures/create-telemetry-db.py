#!/usr/bin/env python3
"""Create a test telemetry.db with known rows for verification probes."""
import sqlite3
import os
from datetime import datetime, timedelta, timezone

db_path = os.path.join(os.path.dirname(__file__), "telemetry.fixture.db")
# Remove existing test DB
if os.path.exists(db_path):
    os.remove(db_path)

conn = sqlite3.connect(db_path)
conn.execute("""
    CREATE TABLE IF NOT EXISTS requests (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        session_id TEXT NOT NULL,
        provider TEXT NOT NULL,
        model TEXT NOT NULL,
        prompt_tokens INTEGER NOT NULL DEFAULT 0,
        completion_tokens INTEGER NOT NULL DEFAULT 0,
        cache_read_tokens INTEGER NOT NULL DEFAULT 0,
        cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
        cost REAL,
        duration_ms INTEGER,
        timestamp TEXT NOT NULL DEFAULT (datetime('now'))
    )
""")

now = datetime(2026, 9, 20, 10, 0, 0, tzinfo=timezone.utc)

rows = [
    # Today's rows (2026-09-20) — multiple models
    ("sess-001", "openrouter", "deepseek/deepseek-v4-pro", 1500, 800, 0, 0, 0.0046, 1200, now),
    ("sess-001", "openrouter", "deepseek/deepseek-v4-pro", 3200, 1500, 500, 0, 0.0123, 2500, now + timedelta(minutes=5)),
    ("sess-002", "local", "qwen-3.8-27b", 800, 400, 0, 0, None, 800, now + timedelta(minutes=10)),
    ("sess-003", "openrouter", "google/gemini-2.5-flash", 500, 200, 0, 0, 0.0002, 300, now + timedelta(hours=1)),
    ("sess-003", "local", "qwen-3.8-27b", 2000, 1000, 0, 0, None, 1500, now + timedelta(hours=2)),
    ("sess-004", "openrouter", "deepseek/deepseek-v4-pro", 5000, 2000, 1000, 0, 0.0150, 3000, now + timedelta(hours=3)),

    # Yesterday (2026-09-19) — for date range testing
    ("sess-005", "openrouter", "deepseek/deepseek-v4-pro", 1000, 500, 0, 0, 0.0030, 600, now - timedelta(days=1)),
    ("sess-005", "local", "qwen-3.8-27b", 600, 300, 0, 0, None, 400, now - timedelta(days=1, hours=-1)),

    # Two days ago (2026-09-18) — for date range testing
    ("sess-006", "openrouter", "google/gemini-2.5-flash", 200, 100, 0, 0, 0.0001, 100, now - timedelta(days=2)),

    # Last week (2026-09-13) — outside default today view
    ("sess-007", "openrouter", "deepseek/deepseek-v4-pro", 10000, 4000, 2000, 0, 0.0300, 5000, now - timedelta(days=7)),
]

for r in rows:
    conn.execute(
        "INSERT INTO requests (session_id, provider, model, prompt_tokens, completion_tokens, cache_read_tokens, cache_creation_tokens, cost, duration_ms, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        (*r[:9], r[9].isoformat())
    )

conn.commit()
conn.close()
print(f"Created {db_path} with {len(rows)} rows")