import io
import unittest

from spikes.native_helper_harness.reference import (
    NativeHelperHarness,
    ProtocolError,
    encode_native_message,
    read_native_message,
    validate_event_shape,
)


def dom_event(session_id: str, epoch: int, **overrides):
    event = {
        "schema_version": "1.0",
        "event_id": "evt_test",
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
            "units": [
                {
                    "unit_id": "u1",
                    "kind": "text",
                    "text": "Visible allowed text",
                    "confidence": 1.0,
                }
            ],
            "capture_method_version": "dom-probe/1",
        },
    }
    event.update(overrides)
    return event


class NativeFramingTests(unittest.TestCase):
    def test_native_message_round_trip(self):
        message = {"protocol_version": "1.0", "kind": "hello", "payload": {"x": "日本語"}}
        encoded = encode_native_message(message)
        self.assertEqual(read_native_message(io.BytesIO(encoded)), message)

    def test_truncated_message_fails(self):
        encoded = encode_native_message({"x": "abc"})
        with self.assertRaisesRegex(ProtocolError, "TRUNCATED_MESSAGE"):
            read_native_message(io.BytesIO(encoded[:-1]))


class EventInvariantTests(unittest.TestCase):
    def test_valid_dom_event_shape(self):
        event = dom_event("ses_test", 1)
        self.assertEqual(validate_event_shape(event), [])

    def test_fingerprint_only_cannot_smuggle_ocr_text(self):
        event = dom_event(
            "ses_test",
            1,
            capture_mode="fingerprint_only",
            payload={"visual_fingerprint": "phash-v1:123", "ocr_text": "secret"},
        )
        errors = validate_event_shape(event)
        self.assertTrue(any("FINGERPRINT_ONLY_CONTENT_FORBIDDEN" in error for error in errors))

    def test_blocked_event_cannot_keep_source_or_content(self):
        event = dom_event(
            "ses_test",
            1,
            capture_mode="blocked",
            payload={"reason": "SITE_EXCLUSION", "text": "must not survive"},
        )
        errors = validate_event_shape(event)
        self.assertIn("BLOCKED_SOURCE_FORBIDDEN", errors)
        self.assertIn("BLOCKED_CONTENT_FORBIDDEN", errors)

    def test_redacted_visual_requires_policy_version(self):
        event = dom_event(
            "ses_test",
            1,
            capture_mode="redacted_visual",
            payload={"units": [], "capture_method_version": "visual/1"},
        )
        self.assertIn("REDACTION_POLICY_REQUIRED", validate_event_shape(event))

    def test_editable_value_field_is_forbidden_even_when_nested(self):
        event = dom_event(
            "ses_test",
            1,
            payload={
                "units": [{"text": "safe", "metadata": {"input_value": "typed secret"}}],
                "capture_method_version": "dom-probe/1",
            },
        )
        errors = validate_event_shape(event)
        self.assertTrue(any(error.startswith("FORBIDDEN_PAYLOAD_FIELD:") for error in errors))


class HelperAuthorityTests(unittest.TestCase):
    def setUp(self):
        self.helper = NativeHelperHarness()
        self.session_id, self.epoch = self.helper.start("ses_test")

    def test_current_epoch_observation_is_accepted(self):
        accepted, errors = self.helper.accept_observation(dom_event(self.session_id, self.epoch))
        self.assertTrue(accepted)
        self.assertEqual(errors, [])

    def test_pause_invalidates_old_epoch(self):
        old_event = dom_event(self.session_id, self.epoch)
        self.helper.pause()
        accepted, errors = self.helper.accept_observation(old_event)
        self.assertFalse(accepted)
        self.assertIn("NOT_RECORDING", errors)
        self.assertIn("STALE_EPOCH", errors)

    def test_resume_uses_new_epoch(self):
        self.helper.pause()
        resumed_epoch = self.helper.resume()
        accepted, errors = self.helper.accept_observation(dom_event(self.session_id, resumed_epoch))
        self.assertTrue(accepted)
        self.assertEqual(errors, [])

    def test_interrupt_fails_closed_and_invalidates_epoch(self):
        old_epoch = self.epoch
        new_epoch = self.helper.interrupt()
        self.assertEqual(self.helper.state, "INTERRUPTED")
        self.assertGreater(new_epoch, old_epoch)
        accepted, errors = self.helper.accept_observation(dom_event(self.session_id, old_epoch))
        self.assertFalse(accepted)
        self.assertIn("NOT_RECORDING", errors)
        self.assertIn("STALE_EPOCH", errors)

    def test_extension_cannot_submit_session_state_as_source_authority(self):
        event = dom_event(
            self.session_id,
            self.epoch,
            event_type="session_state",
            capture_mode=None,
            payload={"from": "IDLE", "to": "RECORDING", "reason": "fake"},
        )
        accepted, errors = self.helper.accept_observation(event)
        self.assertFalse(accepted)
        self.assertIn("STATE_EVENT_HELPER_AUTHORITY", errors)

    def test_protocol_major_mismatch_fails(self):
        response = self.helper.handle_message(
            {"protocol_version": "2.0", "message_id": "m1", "kind": "hello", "payload": {}}
        )
        self.assertFalse(response["payload"]["accepted"])
        self.assertEqual(response["payload"]["reason"], "INCOMPATIBLE_PROTOCOL")

    def test_rejection_ack_does_not_echo_sensitive_payload(self):
        bad_event = dom_event(
            self.session_id,
            self.epoch,
            capture_mode="fingerprint_only",
            payload={"visual_fingerprint": "x", "ocr_text": "sensitive text"},
        )
        response = self.helper.handle_message(
            {
                "protocol_version": "1.0",
                "message_id": "m2",
                "kind": "observation.submit",
                "session_id": self.session_id,
                "recording_epoch": self.epoch,
                "payload": {"event": bad_event},
            }
        )
        self.assertFalse(response["payload"]["accepted"])
        self.assertNotIn("sensitive text", repr(response))


if __name__ == "__main__":
    unittest.main()
