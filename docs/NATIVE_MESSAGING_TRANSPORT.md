# CurioTrace Native Messaging Transport

Status: M1 production transport implementation note. Product/session/privacy semantics remain defined by `docs/NATIVE_HELPER_PROTOCOL.md` and `docs/PRODUCT_SPEC.md`.

## 1. Purpose

The browser extension uses one long-lived Native Messaging connection to the local CurioTrace helper while helper interaction is needed.

The transport is responsible only for:

- opening a browser Native Messaging `Port`;
- sending versioned protocol envelopes;
- correlating replies by `message_id`;
- detecting disconnect/protocol transport failure;
- rejecting pending control operations on failure;
- driving a fresh helper handshake before the connection becomes usable.

It does not decide capture policy, permission policy, observation meaning, or durable storage semantics.

Production modules:

- `apps/extension/src/native-messaging-transport.ts`
- `apps/extension/src/helper-connection.ts`

## 2. Connection model

CurioTrace uses the connection-oriented WebExtension API:

```text
background/service worker
  -> runtime.connectNative(hostName)
  -> Port
  -> hello / hello.ack
  -> correlated control and observation messages
```

Native Messaging must be used from the extension/background context rather than directly from page content scripts.

The concrete browser bootstrap and native-host installation/registration remain separate platform adapters.

## 3. Fail-closed rules

The transport is terminal when any of the following occurs:

- the native `Port` disconnects;
- posting a message throws;
- a pending request exceeds its bounded timeout;
- the helper sends a structurally malformed protocol envelope.

Terminal means:

1. reject every pending request;
2. prevent new sends on that transport instance;
3. disconnect the native port where it is still possible;
4. notify the helper-connection controller;
5. invalidate extension-side helper/capture authority through `onTransportDisconnected()`.

A timeout is deliberately terminal. A helper may have committed a state transition even if its acknowledgement was lost. Continuing on the same logical connection would therefore risk treating an ambiguous authority state as known.

Recovery requires a fresh Native Messaging connection and fresh `hello` handshake. Durable helper authority then decides the safe recovered state.

## 4. Message correlation

Every request has a non-empty protocol `message_id`.

While a request is pending:

- another request with the same ID is rejected locally;
- a valid response with that ID resolves exactly that request;
- unknown/late message IDs are ignored by the baseline transport.

Unknown/late messages are not logged with their payload. A future unsolicited helper-push message requires an explicit versioned protocol capability rather than implicit interpretation in the transport.

## 5. Malformed messages

The transport validates only the minimum envelope needed for safe routing:

- `protocol_version`: string;
- `message_id`: non-empty string;
- `kind`: string;
- `payload`: object;
- optional `session_id`: string;
- optional `recording_epoch`: non-negative integer.

Semantic message validation remains in the protocol/session layers.

A structurally malformed native message terminates the connection rather than being ignored, because it means the extension can no longer establish trustworthy request/reply semantics.

## 6. Handshake controller

`HelperConnectionController` owns the browser-side helper connection generation.

A connection is usable only after:

1. `connectNative()` returns a port;
2. `ExtensionProtocolState.onTransportConnected()` creates a fresh local connection generation;
3. the extension sends `hello`;
4. the helper returns a compatible `hello.ack`;
5. `ExtensionProtocolState.applyHelloAck()` accepts the helper-authoritative state.

An incompatible handshake closes the port and remains fail-closed.

A disconnect event from an old transport instance is ignored after a newer connection has become current. This prevents a delayed old-port event from invalidating a fresh connection.

## 7. Browser differences

Current Chrome and Firefox both expose connection-based Native Messaging through `runtime.connectNative()` and a `runtime.Port` with message/disconnect events.

Error diagnostics differ:

- Firefox exposes `port.error` on an errored disconnect;
- Chrome uses `runtime.lastError` rather than `port.error` for this case.

CurioTrace's transport correctness must not depend on either diagnostic string. The baseline safety action is the same: disconnect means helper authority is no longer trusted locally.

A future browser adapter may surface sanitized error classes for repair UX, but must not log captured protocol payloads.

## 8. Tests

Browser-independent production tests cover:

- response correlation;
- multiple pending requests;
- duplicate pending IDs;
- native disconnect;
- terminal timeout;
- malformed response;
- fresh hello handshake;
- incompatible handshake;
- disconnect invalidating capture authority;
- stale old-port disconnect after reconnection.

Real-browser tests remain required for:

- actual Chrome/Edge/Firefox `runtime.connectNative()` behavior;
- native-host missing/misregistered diagnostics;
- service-worker lifecycle interaction;
- browser shutdown/restart;
- installed native-host manifests and extension IDs.

## 9. References

- `docs/NATIVE_HELPER_PROTOCOL.md`
- `docs/M1_IMPLEMENTATION_PLAN.md`
- Chrome `runtime.connectNative()` / Native Messaging documentation
- MDN `runtime.connectNative()`, `runtime.Port`, and Native Messaging documentation
