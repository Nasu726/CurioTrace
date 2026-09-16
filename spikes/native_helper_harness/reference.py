from __future__ import annotations

import json
import struct
import uuid
from datetime import datetime, timezone
from typing import Any, BinaryIO

PROTOCOL_MAJOR = 1
SCHEMA_MAJOR = 1
MAX_EVENT_BYTES = 256 * 1024
MAX_INBOUND_MESSAGE_BYTES = 64 * 1024 * 1024
MAX_OUTBOUND_MESSAGE_BYTES = 1 * 1024 * 1024

EVENT_TYPES = {
    "session_state",
    "navigation",
    "visibility",
    "content_observation",
    "interaction",
    "privacy_decision",
    "capture_status",
    "gap_error",
}

CAPTURE_MODES = {
    "dom",
    "redacted_visual",
    "fingerprint_only",
    "metadata_only",
    "blocked",
    "failed",
}

FORBIDDEN_KEYS = {
    "form_value",
    "input_value",
    "textarea_value",
    "contenteditable_value",
    "password_value",
    "auth_token",
    "payment_value",
    "security_code",
    "clipboard",
    "clipboard_text",
    "image",
    "image_bytes",
    "raster",
    "raster_base64",
    "thumbnail",
}

CONTENT_KEYS = {
    "text",
    "units",
    "ocr_text",
    "selected_text",
    "image",
    "image_bytes",
    "raster",
    "raster_base64",
    "thumbnail",
}

MODE_ALLOWED_KEYS = {
    "dom": {"units", "capture_method_version", "viewport", "gaps", "uncertainty"},
    "redacted_visual": {
        "units",
        "redaction_policy_version",
        "capture_method_version",
        "viewport",
        "gaps",
        "uncertainty",
    },
    "fingerprint_only": {
        "visual_fingerprint",
        "fingerprint_method_version",
        "viewport",
        "semantic_content_unavailable",
        "reason",
        "change_distance",
    },
    "metadata_only": {"reason", "capture_method_version", "viewport", "gaps", "uncertainty"},
    "blocked": {"reason", "rule_id"},
    "failed": {"reason", "error_code", "capture_method_version", "gap", "uncertainty"},
}


class ProtocolError(ValueError):
    """Raised for malformed or unsafe reference-protocol input."""


def _major(version: Any) -> int:
    try:
        return int(str(version).split(".", 1)[0])
    except (TypeError, ValueError):
        return -1


def _all_keys(value: Any):
    if isinstance(value, dict):
        for key, nested in value.items():
            yield key
            yield from _all_keys(nested)
    elif isinstance(value, list):
        for nested in value:
            yield from _all_keys(nested)


def validate_event_shape(event: dict[str, Any]) -> list[str]:
    """Validate transport-independent M1 observation/privacy invariants.

    The production implementation may use a generated JSON-Schema validator as well.
    This function intentionally duplicates critical privacy checks so they remain
    obvious in tests and do not disappear behind a schema library.
    """

    errors: list[str] = []
    required = {
        "schema_version",
        "event_id",
        "session_id",
        "recording_epoch",
        "event_type",
        "wall_time",
        "monotonic_ms",
        "payload",
    }

    missing = sorted(required - event.keys())
    if missing:
        return [f"MISSING_FIELD:{field}" for field in missing]

    if _major(event["schema_version"]) != SCHEMA_MAJOR:
        errors.append("INCOMPATIBLE_SCHEMA")

    if event["event_type"] not in EVENT_TYPES:
        errors.append("UNKNOWN_EVENT_TYPE")

    if not isinstance(event["event_id"], str) or not event["event_id"]:
        errors.append("INVALID_EVENT_ID")

    if not isinstance(event["session_id"], str) or not event["session_id"]:
        errors.append("INVALID_SESSION_ID")

    if not isinstance(event["recording_epoch"], int) or event["recording_epoch"] < 0:
        errors.append("INVALID_EPOCH")

    if not isinstance(event["monotonic_ms"], (int, float)) or event["monotonic_ms"] < 0:
        errors.append("INVALID_MONOTONIC_TIME")

    try:
        datetime.fromisoformat(str(event["wall_time"]).replace("Z", "+00:00"))
    except ValueError:
        errors.append("INVALID_WALL_TIME")

    payload = event["payload"]
    if not isinstance(payload, dict):
        errors.append("INVALID_PAYLOAD")
    else:
        nested_keys = set(_all_keys(payload))
        forbidden = sorted(nested_keys & FORBIDDEN_KEYS)
        if forbidden:
            errors.append("FORBIDDEN_PAYLOAD_FIELD:" + ",".join(forbidden))

        mode = event.get("capture_mode")
        if mode is not None and mode not in CAPTURE_MODES:
            errors.append("UNKNOWN_CAPTURE_MODE")

        if event["event_type"] == "content_observation" and mode is None:
            errors.append("CAPTURE_MODE_REQUIRED")

        if mode in MODE_ALLOWED_KEYS:
            unexpected = sorted(set(payload) - MODE_ALLOWED_KEYS[mode])
            if unexpected:
                errors.append("UNEXPECTED_MODE_PAYLOAD_FIELD:" + ",".join(unexpected))

        if mode == "blocked":
            if "source" in event:
                errors.append("BLOCKED_SOURCE_FORBIDDEN")
            if nested_keys & CONTENT_KEYS:
                errors.append("BLOCKED_CONTENT_FORBIDDEN")

        if mode in {"fingerprint_only", "metadata_only"} and nested_keys & CONTENT_KEYS:
            errors.append(mode.upper() + "_CONTENT_FORBIDDEN")

        if mode == "redacted_visual" and not isinstance(payload.get("redaction_policy_version"), str):
            errors.append("REDACTION_POLICY_REQUIRED")

    serialized = json.dumps(event, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    if len(serialized) > MAX_EVENT_BYTES:
        errors.append("EVENT_TOO_LARGE")

    return errors


def encode_native_message(message: dict[str, Any], *, max_bytes: int = MAX_OUTBOUND_MESSAGE_BYTES) -> bytes:
    """Encode one Native Messaging JSON message using native-endian 32-bit length."""

    payload = json.dumps(message, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    if len(payload) > max_bytes:
        raise ProtocolError("MESSAGE_TOO_LARGE")
    return struct.pack("=I", len(payload)) + payload


def read_native_message(stream: BinaryIO, *, max_bytes: int = MAX_INBOUND_MESSAGE_BYTES) -> dict[str, Any] | None:
    """Read one Native Messaging message from a binary stream."""

    raw_length = stream.read(4)
    if raw_length == b"":
        return None
    if len(raw_length) != 4:
        raise ProtocolError("TRUNCATED_LENGTH")

    (length,) = struct.unpack("=I", raw_length)
    if length > max_bytes:
        raise ProtocolError("MESSAGE_TOO_LARGE")

    payload = stream.read(length)
    if len(payload) != length:
        raise ProtocolError("TRUNCATED_MESSAGE")

    try:
        value = json.loads(payload.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ProtocolError("INVALID_JSON") from error

    if not isinstance(value, dict):
        raise ProtocolError("MESSAGE_MUST_BE_OBJECT")
    return value


def write_native_message(stream: BinaryIO, message: dict[str, Any]) -> None:
    stream.write(encode_native_message(message))
    stream.flush()


class NativeHelperHarness:
    """Reference state authority for protocol/invariant tests only."""

    def __init__(self) -> None:
        self.state = "IDLE"
        self.session_id: str | None = None
        self.recording_epoch = 0
        self.events: list[dict[str, Any]] = []

    @staticmethod
    def _event_id() -> str:
        return "evt_" + uuid.uuid4().hex

    @staticmethod
    def _utc_now() -> str:
        return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    def _append_state_event(self, old: str, new: str, reason: str) -> None:
        assert self.session_id is not None
        self.events.append(
            {
                "schema_version": "1.0",
                "event_id": self._event_id(),
                "session_id": self.session_id,
                "recording_epoch": self.recording_epoch,
                "event_type": "session_state",
                "wall_time": self._utc_now(),
                "monotonic_ms": 0.0,
                "payload": {"from": old, "to": new, "reason": reason},
            }
        )

    def start(self, session_id: str | None = None) -> tuple[str, int]:
        if self.state not in {"IDLE", "FINISHED"}:
            raise ProtocolError("INVALID_TRANSITION")

        old = self.state
        self.session_id = session_id or "ses_" + uuid.uuid4().hex
        self.recording_epoch += 1
        self.state = "RECORDING"
        self._append_state_event(old, self.state, "USER_START")
        return self.session_id, self.recording_epoch

    def pause(self) -> int:
        if self.state != "RECORDING":
            raise ProtocolError("INVALID_TRANSITION")
        old = self.state
        self.recording_epoch += 1
        self.state = "PAUSED"
        self._append_state_event(old, self.state, "USER_PAUSE")
        return self.recording_epoch

    def resume(self) -> int:
        if self.state not in {"PAUSED", "INTERRUPTED"}:
            raise ProtocolError("INVALID_TRANSITION")
        old = self.state
        self.recording_epoch += 1
        self.state = "RECORDING"
        self._append_state_event(old, self.state, "USER_RESUME")
        return self.recording_epoch

    def stop(self) -> int:
        if self.state not in {"RECORDING", "PAUSED", "INTERRUPTED"}:
            raise ProtocolError("INVALID_TRANSITION")
        old = self.state
        self.recording_epoch += 1
        self.state = "FINISHED"
        self._append_state_event(old, self.state, "USER_STOP")
        return self.recording_epoch

    def interrupt(self, reason: str = "HELPER_DISCONNECT") -> int:
        if self.state not in {"RECORDING", "PAUSED"}:
            return self.recording_epoch
        old = self.state
        self.recording_epoch += 1
        self.state = "INTERRUPTED"
        self._append_state_event(old, self.state, reason)
        return self.recording_epoch

    def accept_observation(self, event: dict[str, Any]) -> tuple[bool, list[str]]:
        errors = validate_event_shape(event)

        if event.get("event_type") == "session_state":
            errors.append("STATE_EVENT_HELPER_AUTHORITY")

        if self.state != "RECORDING":
            errors.append("NOT_RECORDING")

        if event.get("session_id") != self.session_id:
            errors.append("UNKNOWN_SESSION")

        if event.get("recording_epoch") != self.recording_epoch:
            errors.append("STALE_EPOCH")

        errors = sorted(set(errors))
        if errors:
            return False, errors

        self.events.append(event)
        return True, []

    def handle_message(self, message: dict[str, Any]) -> dict[str, Any]:
        if _major(message.get("protocol_version")) != PROTOCOL_MAJOR:
            return self._reply(message, False, "INCOMPATIBLE_PROTOCOL")

        kind = message.get("kind")
        payload = message.get("payload") or {}

        if kind == "hello":
            return {
                "protocol_version": "1.0",
                "message_id": message.get("message_id"),
                "kind": "hello.ack",
                "payload": {
                    "compatible": True,
                    "helper_capabilities": [
                        "observation_schema_v1",
                        "recording_epoch_v1",
                        "fail_closed_disconnect_v1",
                    ],
                    "session_state": self.state,
                    "session_id": self.session_id,
                    "recording_epoch": self.recording_epoch,
                },
            }

        if kind == "session.start":
            try:
                session_id, epoch = self.start()
            except ProtocolError as error:
                return self._reply(message, False, str(error))
            return self._reply(
                message,
                True,
                "OK",
                {"session_id": session_id, "recording_epoch": epoch, "state": self.state},
            )

        if kind in {"session.pause", "session.resume", "session.stop"}:
            if message.get("session_id") != self.session_id:
                return self._reply(message, False, "UNKNOWN_SESSION")
            if message.get("recording_epoch") != self.recording_epoch:
                return self._reply(message, False, "STALE_EPOCH")
            try:
                if kind == "session.pause":
                    epoch = self.pause()
                elif kind == "session.resume":
                    epoch = self.resume()
                else:
                    epoch = self.stop()
            except ProtocolError as error:
                return self._reply(message, False, str(error))
            return self._reply(message, True, "OK", {"recording_epoch": epoch, "state": self.state})

        if kind == "observation.submit":
            event = payload.get("event")
            if not isinstance(event, dict):
                return self._reply(message, False, "INVALID_SCHEMA")
            accepted, errors = self.accept_observation(event)
            return self._reply(
                message,
                accepted,
                "OK" if accepted else errors[0],
                {"errors": errors},
            )

        return self._reply(message, False, "UNKNOWN_MESSAGE_KIND")

    @staticmethod
    def _reply(
        message: dict[str, Any],
        accepted: bool,
        reason: str,
        extra: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"accepted": accepted, "reason": reason}
        if extra:
            payload.update(extra)
        return {
            "protocol_version": "1.0",
            "message_id": message.get("message_id"),
            "kind": "ack",
            "payload": payload,
        }
