export type NavigationPrivacyReason =
  | "PRIVATE_BROWSING"
  | "PRIVATE_STATUS_UNKNOWN"
  | "BROWSER_INTERNAL_SURFACE"
  | "USER_EXCLUSION"
  | "INVALID_URL";

export type NavigationEligibility =
  | {
      allowed: true;
      source?: {
        url: string;
        title?: string;
      };
      sourceOmittedReason?: "URL_UNAVAILABLE" | "URL_TOO_LONG";
    }
  | {
      allowed: false;
      reason: NavigationPrivacyReason;
      ruleId?: string;
    };

export interface NavigationContext {
  tabId: number;
  incognito?: boolean;
  url?: string;
  title?: string;
}

export interface UserExclusionMatch {
  excluded: boolean;
  ruleId?: string;
}

export interface UserExclusionPolicy {
  match(tabId: number, url: URL): UserExclusionMatch;
  clearTab?(tabId: number): void;
}

export class InMemoryUserExclusionPolicy implements UserExclusionPolicy {
  #tabRules = new Map<number, string>();
  #pageRules = new Map<string, string>();
  #domainRules = new Map<string, string>();
  #ruleSequence = 0;

  excludeTab(tabId: number, ruleId = this.#nextRuleId("tab")): string {
    this.#tabRules.set(tabId, ruleId);
    return ruleId;
  }

  excludePage(rawUrl: string, ruleId = this.#nextRuleId("page")): string {
    const parsed = parseHttpUrl(rawUrl);
    if (!parsed) {
      throw new Error("page exclusion requires an HTTP/HTTPS URL");
    }
    this.#pageRules.set(comparableUrl(parsed), ruleId);
    return ruleId;
  }

  excludeDomain(rawDomain: string, ruleId = this.#nextRuleId("domain")): string {
    const domain = normalizeDomain(rawDomain);
    if (!domain) {
      throw new Error("domain exclusion requires a valid hostname");
    }
    this.#domainRules.set(domain, ruleId);
    return ruleId;
  }

  clearTab(tabId: number): void {
    this.#tabRules.delete(tabId);
  }

  match(tabId: number, url: URL): UserExclusionMatch {
    const tabRule = this.#tabRules.get(tabId);
    if (tabRule) {
      return { excluded: true, ruleId: tabRule };
    }

    const pageRule = this.#pageRules.get(comparableUrl(url));
    if (pageRule) {
      return { excluded: true, ruleId: pageRule };
    }

    const hostname = url.hostname.toLowerCase();
    let bestDomain = "";
    let bestRule: string | undefined;
    for (const [domain, ruleId] of this.#domainRules) {
      if ((hostname === domain || hostname.endsWith(`.${domain}`)) && domain.length > bestDomain.length) {
        bestDomain = domain;
        bestRule = ruleId;
      }
    }
    if (bestRule) {
      return { excluded: true, ruleId: bestRule };
    }
    return { excluded: false };
  }

  #nextRuleId(scope: string): string {
    this.#ruleSequence += 1;
    return `exclude_${scope}_${this.#ruleSequence}`;
  }
}

export class NavigationPrivacyGate {
  readonly exclusions: UserExclusionPolicy;

  constructor(exclusions: UserExclusionPolicy = new InMemoryUserExclusionPolicy()) {
    this.exclusions = exclusions;
  }

  evaluate(context: NavigationContext): NavigationEligibility {
    if (context.incognito === true) {
      return { allowed: false, reason: "PRIVATE_BROWSING" };
    }
    if (context.incognito !== false) {
      return { allowed: false, reason: "PRIVATE_STATUS_UNKNOWN" };
    }
    if (!context.url) {
      return { allowed: true, sourceOmittedReason: "URL_UNAVAILABLE" };
    }

    const parsed = parseHttpUrl(context.url);
    if (!parsed) {
      try {
        new URL(context.url);
        return { allowed: false, reason: "BROWSER_INTERNAL_SURFACE" };
      } catch {
        return { allowed: false, reason: "INVALID_URL" };
      }
    }

    const exclusion = this.exclusions.match(context.tabId, parsed);
    if (exclusion.excluded) {
      return {
        allowed: false,
        reason: "USER_EXCLUSION",
        ...(exclusion.ruleId ? { ruleId: exclusion.ruleId } : {}),
      };
    }

    parsed.username = "";
    parsed.password = "";
    const safeUrl = parsed.toString();
    if (safeUrl.length > 8192) {
      return { allowed: true, sourceOmittedReason: "URL_TOO_LONG" };
    }

    const source: { url: string; title?: string } = { url: safeUrl };
    if (typeof context.title === "string" && context.title.length > 0 && context.title.length <= 2048) {
      source.title = context.title;
    }
    return { allowed: true, source };
  }
}

function parseHttpUrl(rawUrl: string): URL | null {
  let parsed: URL;
  try {
    parsed = new URL(rawUrl);
  } catch {
    return null;
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return null;
  }
  return parsed;
}

function comparableUrl(url: URL): string {
  const copy = new URL(url.toString());
  copy.username = "";
  copy.password = "";
  return copy.toString();
}

function normalizeDomain(rawDomain: string): string {
  const trimmed = rawDomain.trim().toLowerCase().replace(/^\.+/, "").replace(/\.+$/, "");
  if (!trimmed || trimmed.includes("/") || trimmed.includes(":")) {
    return "";
  }
  try {
    const parsed = new URL(`https://${trimmed}/`);
    return parsed.hostname.toLowerCase();
  } catch {
    return "";
  }
}
