import assert from "node:assert/strict";
import test from "node:test";

import {
  InMemoryUserExclusionPolicy,
  NavigationPrivacyGate,
} from "../dist/navigation-privacy-gate.js";

test("private and unknown-private contexts fail closed before source persistence", () => {
  const gate = new NavigationPrivacyGate();
  assert.deepEqual(
    gate.evaluate({ tabId: 1, incognito: true, url: "https://secret.example/" }),
    { allowed: false, reason: "PRIVATE_BROWSING" },
  );
  assert.deepEqual(
    gate.evaluate({ tabId: 1, url: "https://secret.example/" }),
    { allowed: false, reason: "PRIVATE_STATUS_UNKNOWN" },
  );
});

test("browser-internal and malformed destinations are denied", () => {
  const gate = new NavigationPrivacyGate();
  assert.deepEqual(
    gate.evaluate({ tabId: 2, incognito: false, url: "chrome://settings/" }),
    { allowed: false, reason: "BROWSER_INTERNAL_SURFACE" },
  );
  assert.deepEqual(
    gate.evaluate({ tabId: 2, incognito: false, url: "not a url" }),
    { allowed: false, reason: "INVALID_URL" },
  );
});

test("tab page and domain exclusions return only non-content rule identity", () => {
  const exclusions = new InMemoryUserExclusionPolicy();
  const gate = new NavigationPrivacyGate(exclusions);
  exclusions.excludeDomain("example.test", "domain_rule");
  assert.deepEqual(
    gate.evaluate({ tabId: 3, incognito: false, url: "https://sub.example.test/private?q=secret", title: "Secret" }),
    { allowed: false, reason: "USER_EXCLUSION", ruleId: "domain_rule" },
  );

  exclusions.excludeTab(4, "tab_rule");
  assert.deepEqual(
    gate.evaluate({ tabId: 4, incognito: false, url: "https://other.test/" }),
    { allowed: false, reason: "USER_EXCLUSION", ruleId: "tab_rule" },
  );
});

test("allowed source strips URL credentials and redacts obvious secret query values", () => {
  const gate = new NavigationPrivacyGate();
  const result = gate.evaluate({
    tabId: 5,
    incognito: false,
    url: "https://user:pass@example.test/callback?code=oauth-secret&article=42&access_token=token-secret",
    title: "Callback",
  });
  assert.equal(result.allowed, true);
  assert.ok(result.source);
  assert.equal(result.source.url.includes("user:pass"), false);
  assert.equal(result.source.url.includes("oauth-secret"), false);
  assert.equal(result.source.url.includes("token-secret"), false);
  assert.equal(result.source.url.includes("article=42"), true);
  assert.equal(result.source.url.includes("code=__redacted__"), true);
  assert.equal(result.source.url.includes("access_token=__redacted__"), true);
});

test("secret-bearing URL fragment is removed while ordinary fragment is retained", () => {
  const gate = new NavigationPrivacyGate();
  const secret = gate.evaluate({
    tabId: 6,
    incognito: false,
    url: "https://example.test/callback#access_token=fragment-secret&state=abc",
  });
  assert.equal(secret.allowed, true);
  assert.ok(secret.source);
  assert.equal(secret.source.url.includes("fragment-secret"), false);
  assert.equal(secret.source.url.includes("#"), false);

  const ordinary = gate.evaluate({
    tabId: 6,
    incognito: false,
    url: "https://example.test/article#section-2",
  });
  assert.equal(ordinary.allowed, true);
  assert.equal(ordinary.source.url.endsWith("#section-2"), true);
});

test("page exclusion comparison uses the same secret-redacted URL representation", () => {
  const exclusions = new InMemoryUserExclusionPolicy();
  const gate = new NavigationPrivacyGate(exclusions);
  exclusions.excludePage("https://example.test/callback?code=first-secret&x=1", "page_rule");

  const result = gate.evaluate({
    tabId: 7,
    incognito: false,
    url: "https://example.test/callback?code=second-secret&x=1",
  });
  assert.deepEqual(result, { allowed: false, reason: "USER_EXCLUSION", ruleId: "page_rule" });
});
