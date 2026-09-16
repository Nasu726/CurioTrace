#!/usr/bin/env python3
"""Disposable Native Messaging reference host for CurioTrace protocol tests.

Not production code. The process keeps state in memory and intentionally writes
no captured payloads to logs/files.
"""

from __future__ import annotations

import sys

from .reference import NativeHelperHarness, ProtocolError, read_native_message, write_native_message


def main() -> int:
    helper = NativeHelperHarness()
    stdin = sys.stdin.buffer
    stdout = sys.stdout.buffer

    while True:
        try:
            message = read_native_message(stdin)
        except ProtocolError:
            # Native Messaging stdout must contain protocol frames only.
            # Fail closed rather than attempting to echo malformed input.
            helper.interrupt("PROTOCOL_ERROR")
            return 2

        if message is None:
            helper.interrupt("NATIVE_PORT_EOF")
            return 0

        response = helper.handle_message(message)
        try:
            write_native_message(stdout, response)
        except (BrokenPipeError, ProtocolError):
            helper.interrupt("NATIVE_PORT_WRITE_FAILURE")
            return 3


if __name__ == "__main__":
    raise SystemExit(main())
