from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any

from .reference import NativeHelperHarness


class ReferenceSqliteStore:
    """Reference-only durable store for M1 lifecycle semantics.

    This is deliberately **not** the production encryption/storage decision.
    It stores only already-validated compact observation events and helper state.
    """

    def __init__(self, path: str | Path) -> None:
        self.path = str(path)
        self.connection = sqlite3.connect(self.path)
        self.connection.row_factory = sqlite3.Row
        self.connection.execute("PRAGMA foreign_keys = ON")
        self.connection.executescript(
            """
            CREATE TABLE IF NOT EXISTS sessions (
                session_id TEXT PRIMARY KEY,
                state TEXT NOT NULL,
                recording_epoch INTEGER NOT NULL,
                updated_seq INTEGER NOT NULL
            );

            CREATE TABLE IF NOT EXISTS events (
                event_id TEXT PRIMARY KEY,
                session_id TEXT NOT NULL,
                recording_epoch INTEGER NOT NULL,
                event_type TEXT NOT NULL,
                wall_time TEXT NOT NULL,
                monotonic_ms REAL NOT NULL,
                capture_mode TEXT,
                event_json TEXT NOT NULL,
                FOREIGN KEY(session_id) REFERENCES sessions(session_id) ON DELETE CASCADE
            );

            CREATE INDEX IF NOT EXISTS idx_events_session_time
            ON events(session_id, monotonic_ms, event_id);
            """
        )
        self.connection.commit()

    def close(self) -> None:
        self.connection.close()

    def save_session(self, session_id: str, state: str, recording_epoch: int) -> None:
        next_seq = self.connection.execute(
            "SELECT COALESCE(MAX(updated_seq), 0) + 1 FROM sessions"
        ).fetchone()[0]
        self.connection.execute(
            """
            INSERT INTO sessions(session_id, state, recording_epoch, updated_seq)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(session_id) DO UPDATE SET
                state = excluded.state,
                recording_epoch = excluded.recording_epoch,
                updated_seq = excluded.updated_seq
            """,
            (session_id, state, recording_epoch, next_seq),
        )
        self.connection.commit()

    def append_event(self, event: dict[str, Any]) -> None:
        self.connection.execute(
            """
            INSERT INTO events(
                event_id,
                session_id,
                recording_epoch,
                event_type,
                wall_time,
                monotonic_ms,
                capture_mode,
                event_json
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                event["event_id"],
                event["session_id"],
                event["recording_epoch"],
                event["event_type"],
                event["wall_time"],
                event["monotonic_ms"],
                event.get("capture_mode"),
                json.dumps(event, ensure_ascii=False, separators=(",", ":")),
            ),
        )
        self.connection.commit()

    def latest_unfinished_session(self) -> dict[str, Any] | None:
        row = self.connection.execute(
            """
            SELECT session_id, state, recording_epoch
            FROM sessions
            WHERE state != 'FINISHED'
            ORDER BY updated_seq DESC
            LIMIT 1
            """
        ).fetchone()
        return dict(row) if row else None

    def event_count(self, session_id: str) -> int:
        return self.connection.execute(
            "SELECT COUNT(*) FROM events WHERE session_id = ?", (session_id,)
        ).fetchone()[0]

    def events_for_session(self, session_id: str) -> list[dict[str, Any]]:
        rows = self.connection.execute(
            """
            SELECT event_json
            FROM events
            WHERE session_id = ?
            ORDER BY rowid
            """,
            (session_id,),
        ).fetchall()
        return [json.loads(row["event_json"]) for row in rows]


class PersistentNativeHelperHarness:
    """Persistence wrapper around the in-memory protocol authority.

    It demonstrates the M1 contract that a process restart must never silently
    restore RECORDING. Any unfinished stored session becomes INTERRUPTED with a
    fresh epoch before the extension can obtain new capture authority.
    """

    def __init__(self, store: ReferenceSqliteStore) -> None:
        self.store = store
        self.helper = NativeHelperHarness()
        self._flushed_events = 0
        self._recover_unfinished_session()

    @property
    def state(self) -> str:
        return self.helper.state

    @property
    def session_id(self) -> str | None:
        return self.helper.session_id

    @property
    def recording_epoch(self) -> int:
        return self.helper.recording_epoch

    def _recover_unfinished_session(self) -> None:
        stored = self.store.latest_unfinished_session()
        if stored is None:
            return

        old_state = stored["state"]
        self.helper.session_id = stored["session_id"]
        self.helper.recording_epoch = int(stored["recording_epoch"]) + 1
        self.helper.state = "INTERRUPTED"
        self.helper._append_state_event(old_state, "INTERRUPTED", "HELPER_PROCESS_RESTART")
        self._sync()

    def _sync(self) -> None:
        if self.helper.session_id is not None:
            self.store.save_session(
                self.helper.session_id,
                self.helper.state,
                self.helper.recording_epoch,
            )

        while self._flushed_events < len(self.helper.events):
            event = self.helper.events[self._flushed_events]
            self.store.append_event(event)
            self._flushed_events += 1

    def start(self, session_id: str | None = None) -> tuple[str, int]:
        result = self.helper.start(session_id)
        self._sync()
        return result

    def pause(self) -> int:
        epoch = self.helper.pause()
        self._sync()
        return epoch

    def resume(self) -> int:
        epoch = self.helper.resume()
        self._sync()
        return epoch

    def stop(self) -> int:
        epoch = self.helper.stop()
        self._sync()
        return epoch

    def interrupt(self, reason: str = "HELPER_DISCONNECT") -> int:
        epoch = self.helper.interrupt(reason)
        self._sync()
        return epoch

    def accept_observation(self, event: dict[str, Any]) -> tuple[bool, list[str]]:
        accepted, errors = self.helper.accept_observation(event)
        if accepted:
            self._sync()
        return accepted, errors

    def handle_message(self, message: dict[str, Any]) -> dict[str, Any]:
        response = self.helper.handle_message(message)
        self._sync()
        return response
