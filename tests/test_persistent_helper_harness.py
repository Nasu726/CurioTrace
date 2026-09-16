import tempfile
import unittest
from pathlib import Path

from spikes.native_helper_harness.persistent import (
    PersistentNativeHelperHarness,
    ReferenceSqliteStore,
)


def dom_event(session_id: str, epoch: int, event_id: str = "evt_obs"):
    return {
        "schema_version": "1.0",
        "event_id": event_id,
        "session_id": session_id,
        "recording_epoch": epoch,
        "event_type": "content_observation",
        "wall_time": "2026-09-16T09:30:00Z",
        "monotonic_ms": 1000.0,
        "browser_instance_id": "br_test",
        "view_id": "view_test",
        "capture_mode": "dom",
        "source": {"url": "https://example.test/", "title": "Example"},
        "payload": {
            "units": [{"unit_id": "u1", "kind": "text", "text": "allowed text", "confidence": 1.0}],
            "capture_method_version": "dom-probe/1",
        },
    }


class PersistentHelperTests(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.db_path = Path(self.tempdir.name) / "reference.sqlite3"

    def tearDown(self):
        self.tempdir.cleanup()

    def open_store(self):
        return ReferenceSqliteStore(self.db_path)

    def test_valid_observation_survives_store_reopen(self):
        store = self.open_store()
        helper = PersistentNativeHelperHarness(store)
        session_id, epoch = helper.start("ses_persist")
        accepted, errors = helper.accept_observation(dom_event(session_id, epoch))
        self.assertTrue(accepted)
        self.assertEqual(errors, [])
        self.assertEqual(store.event_count(session_id), 2)  # start + observation
        store.close()

        reopened = self.open_store()
        events = reopened.events_for_session(session_id)
        self.assertEqual(len(events), 2)
        self.assertEqual(events[-1]["payload"]["units"][0]["text"], "allowed text")
        reopened.close()

    def test_recording_process_restart_becomes_interrupted_with_new_epoch(self):
        store = self.open_store()
        first = PersistentNativeHelperHarness(store)
        session_id, old_epoch = first.start("ses_crash")
        old_event = dom_event(session_id, old_epoch, "evt_old")
        store.close()  # simulate helper process disappearance without a clean transition

        reopened_store = self.open_store()
        recovered = PersistentNativeHelperHarness(reopened_store)
        self.assertEqual(recovered.state, "INTERRUPTED")
        self.assertEqual(recovered.session_id, session_id)
        self.assertGreater(recovered.recording_epoch, old_epoch)

        accepted, errors = recovered.accept_observation(old_event)
        self.assertFalse(accepted)
        self.assertIn("NOT_RECORDING", errors)
        self.assertIn("STALE_EPOCH", errors)

        events = reopened_store.events_for_session(session_id)
        self.assertEqual(events[-1]["event_type"], "session_state")
        self.assertEqual(events[-1]["payload"]["to"], "INTERRUPTED")
        self.assertEqual(events[-1]["payload"]["reason"], "HELPER_PROCESS_RESTART")
        reopened_store.close()

    def test_paused_process_restart_also_remains_fail_closed(self):
        store = self.open_store()
        first = PersistentNativeHelperHarness(store)
        session_id, _ = first.start("ses_paused")
        paused_epoch = first.pause()
        store.close()

        reopened_store = self.open_store()
        recovered = PersistentNativeHelperHarness(reopened_store)
        self.assertEqual(recovered.state, "INTERRUPTED")
        self.assertGreater(recovered.recording_epoch, paused_epoch)
        reopened_store.close()

    def test_explicit_resume_after_recovery_uses_another_fresh_epoch(self):
        store = self.open_store()
        first = PersistentNativeHelperHarness(store)
        session_id, old_epoch = first.start("ses_resume")
        store.close()

        reopened_store = self.open_store()
        recovered = PersistentNativeHelperHarness(reopened_store)
        interrupted_epoch = recovered.recording_epoch
        resumed_epoch = recovered.resume()
        self.assertGreater(interrupted_epoch, old_epoch)
        self.assertGreater(resumed_epoch, interrupted_epoch)
        self.assertEqual(recovered.state, "RECORDING")

        accepted, errors = recovered.accept_observation(
            dom_event(session_id, resumed_epoch, "evt_after_resume")
        )
        self.assertTrue(accepted)
        self.assertEqual(errors, [])
        reopened_store.close()

    def test_finished_session_is_not_resurrected_after_restart(self):
        store = self.open_store()
        first = PersistentNativeHelperHarness(store)
        first.start("ses_finished")
        first.stop()
        store.close()

        reopened_store = self.open_store()
        recovered = PersistentNativeHelperHarness(reopened_store)
        self.assertEqual(recovered.state, "IDLE")
        self.assertIsNone(recovered.session_id)
        reopened_store.close()

    def test_privacy_invalid_event_is_never_persisted(self):
        store = self.open_store()
        helper = PersistentNativeHelperHarness(store)
        session_id, epoch = helper.start("ses_privacy")
        before = store.event_count(session_id)

        invalid = dom_event(session_id, epoch, "evt_invalid")
        invalid["payload"]["input_value"] = "typed secret"
        accepted, errors = helper.accept_observation(invalid)

        self.assertFalse(accepted)
        self.assertTrue(any(error.startswith("FORBIDDEN_PAYLOAD_FIELD:") for error in errors))
        self.assertEqual(store.event_count(session_id), before)
        self.assertNotIn("typed secret", repr(store.events_for_session(session_id)))
        store.close()


if __name__ == "__main__":
    unittest.main()
