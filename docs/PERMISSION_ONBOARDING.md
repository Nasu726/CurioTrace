# CurioTrace First-Start Permission Onboarding

Status: canonical product semantics for #26. Localized/polished UI text may vary, but the claims below must remain true.

## 1. Why the permission is broad

A CurioTrace recording session can move across arbitrary normal HTTP/HTTPS sites and tabs. To avoid creating a session that silently loses pages whenever the user changes origin, CurioTrace needs browser host access broad enough to observe allowed pages throughout the explicitly started session.

The permission is requested at the **first explicit Start**, not merely because the extension was installed.

If the required permission is not granted, CurioTrace does **not** enter `RECORDING` and does not create a partial session that looks complete.

## 2. Required explanation before the browser prompt

Before invoking the browser-generated host-permission prompt, CurioTrace explains all of the following in plain language:

- **Why:** access is needed so one recording session can remain complete as the user moves between normal websites/tabs.
- **When capture happens:** only after an explicit `Start` while the session is in `RECORDING`.
- **When capture does not happen:** before Start, while Paused, after Stop, and on excluded/private contexts according to the privacy policy.
- **What may be handled locally:** allowed page/source metadata, visible allowed content, exposure/interaction observations, and transient viewport data as defined by the capture/privacy policy.
- **What the browser permission does not mean:** granting host access does not itself start always-on monitoring.
- **External AI is separate:** host permission is not authorization to send browsing data to an external LLM/provider. External summarization has its own authorization boundary.
- **Refusal behavior:** declining/cancelling means recording does not start; CurioTrace does not silently fall back to incomplete cross-site tracking.

Do not claim that CurioTrace can perfectly recognize arbitrary secrets/confidential information rendered as ordinary allowed page content.

## 3. Canonical short copy

A baseline English copy is:

> CurioTrace needs access to normal web pages while a recording session is active so it can keep the same session complete as you move between sites and tabs. Granting this browser permission does not start background monitoring: CurioTrace reads browsing content only after you press Start and stops while Paused or after Stop. Private and excluded pages remain outside capture under the privacy policy. This permission also does not authorize sending your browsing data to an external AI service.

Primary action:

> Continue to browser permission

Secondary action:

> Cancel

If permission is declined/cancelled:

> Recording was not started because CurioTrace does not create partial cross-site sessions without the required website access. You can try Start again later.

## 4. UX rules

- The CurioTrace explanation appears **before** the browser's own permission prompt.
- Do not hide the broad scope behind generic wording such as “Continue”.
- The explanation should be readable without opening the full privacy policy, while linking to it when available.
- The browser's permission UI remains authoritative for granting the browser capability; CurioTrace must not imitate or spoof browser chrome.
- Permission already granted: subsequent Starts need not repeat the full explanation unless scope/policy materially changes.
- If permission has been revoked since the previous session, Start returns to the explanation/request flow.
- A permission request failure, cancellation, or rejection leaves the app out of `RECORDING`.

## 5. Relationship to session state

Browser host permission and recording authorization are different layers:

```text
host permission absent + IDLE      -> cannot Start
host permission granted + IDLE     -> no capture
host permission granted + RECORDING -> allowed capture subject to privacy rules
host permission granted + PAUSED   -> no content capture
host permission granted + FINISHED -> no content capture
```

The host permission may remain granted between sessions. This is a browser capability only; CurioTrace's explicit session state is the actual capture authorization.

## 6. Store/privacy consistency

Chrome Web Store / Firefox Add-ons disclosures, the public privacy policy, and in-product wording must describe the same behavior.

In particular:

- do not describe Native Messaging/local helper processing as if no data leaves the add-on context when a store policy treats it as transmission;
- distinguish local helper processing from optional external frontier-model transmission;
- disclose the data classes actually handled by the implementation;
- do not add telemetry involving browsing-derived content/activity without a new review of disclosure/consent requirements.

See #22 and `docs/PRODUCT_SPEC.md`.

## 7. Production implementation status

The browser-independent production state machine is implemented in:

- `apps/extension/src/permission-controller.ts`

It currently enforces:

- exact required runtime host origins `http://*/*` and `https://*/*`;
- a permission check on every Start attempt, so permission revoked between sessions is detected;
- no browser permission request before the CurioTrace explanation state has been reached and the user explicitly chooses the Continue action;
- no helper/session Start call after denial, cancellation, permission API failure, or an ineffective grant;
- rechecking the permission before helper Start;
- skipping repeated explanation/request when the permission is already granted;
- fail-closed handling of concurrent Start attempts and helper Start rejection/failure.

`WebExtensionHostPermissionPort` is a thin Promise-based adapter for the current Chromium/Firefox permissions API shape. The concrete extension manifest, popup/onboarding UI, user-action event wiring, and browser-level permission integration tests are still pending. Until those adapters are wired and tested, this section does not claim that the production browser extension can yet perform the complete first-Start flow.
