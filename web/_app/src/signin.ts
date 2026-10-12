// Signing in, hosted (chronicle-agents-hq design 03 § signing in hosted,
// decision 0007): the console's flow, ported — GitHub's web flow with a
// state and PKCE, the code traded at the install's control, the tokens
// kept in the tab and refreshed before they lapse. The primitives are
// chronicle-js's; nothing of GitHub's is reimplemented here. The pure
// parts — reading GitHub's return, choosing an account — take their
// inputs as values so the tests need no window.
import {
  exchangeGithubCode,
  githubAuthorizeURL,
  pkcePair,
  randomToken,
  refreshGithubToken,
  type GithubTokens,
  type IdentityMembership,
  type InstallProfile,
} from "@impire-io/chronicle";

const PENDING = "chronicle-agents.signin.pending";
const TOKENS = "chronicle-agents.signin.tokens";
const LAST_ACCOUNT = "chronicle-agents.account";
/** Refresh this long before the access token lapses. */
const REFRESH_AHEAD_MS = 5 * 60_000;

/** The tokens the page holds. */
export interface Tokens {
  access: string;
  refresh?: string;
  /** When the access token lapses, epoch milliseconds; undefined when GitHub said nothing. */
  expiresAt?: number;
}

interface Pending {
  state: string;
  verifier: string;
  redirectUri: string;
}

/** Where GitHub sends the browser back: the interface's own address. */
export function redirectURI(): string {
  return `${window.location.origin}/`;
}

function read<T>(key: string): T | undefined {
  try {
    const raw = window.sessionStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : undefined;
  } catch {
    return undefined;
  }
}

function write(key: string, value: unknown): void {
  try {
    if (value === undefined) window.sessionStorage.removeItem(key);
    else window.sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Storage refused (a private window): the sign-in holds for this page's life only.
  }
}

const fromGrant = (g: GithubTokens): Tokens => ({
  access: g.access_token,
  ...(g.refresh_token ? { refresh: g.refresh_token } : {}),
  ...(g.expires_in ? { expiresAt: Date.now() + g.expires_in * 1000 } : {}),
});

let memory: Tokens | undefined;

/** The tokens held now, if any. */
export function heldTokens(): Tokens | undefined {
  return memory ?? read<Tokens>(TOKENS);
}

function hold(tokens: Tokens | undefined): void {
  memory = tokens;
  write(TOKENS, tokens);
}

/** Sends the browser to GitHub to sign in. */
export async function startSignIn(profile: InstallProfile): Promise<void> {
  const { verifier, challenge } = await pkcePair();
  const pending: Pending = { state: randomToken(), verifier, redirectUri: redirectURI() };
  write(PENDING, pending);
  window.location.assign(
    githubAuthorizeURL(profile, { redirectUri: pending.redirectUri, state: pending.state, codeChallenge: challenge }),
  );
}

/** What GitHub's return to the page says, before any exchange. */
export type Return =
  | { kind: "none" }
  | { kind: "code"; code: string; pending: Pending }
  | { kind: "refused"; why: string };

/**
 * Reads a return from GitHub out of the address: nothing when the page
 * was not sent back, the code when the state is the one this tab sent,
 * a refusal otherwise. Pure: the tests hand it the search and the pending
 * sign-in.
 */
export function readReturn(search: string, pending: Pending | undefined): Return {
  const q = new URLSearchParams(search);
  const code = q.get("code");
  const state = q.get("state");
  const error = q.get("error_description") ?? q.get("error");
  if (!code && !error) return { kind: "none" };
  if (error) return { kind: "refused", why: `GitHub said: ${error}` };
  if (!pending || pending.state !== state || !code) {
    return { kind: "refused", why: "this return from GitHub is not the sign-in this tab started; sign in again" };
  }
  return { kind: "code", code, pending };
}

/** The outcome of completing a sign-in. */
export type Completed = { kind: "none" } | { kind: "signed-in"; tokens: Tokens } | { kind: "refused"; why: string };

/**
 * Completes a sign-in when the page is GitHub's return: the code is traded
 * at control, and the address loses the code either way.
 */
export async function completeSignIn(profile: InstallProfile): Promise<Completed> {
  const url = new URL(window.location.href);
  const ret = readReturn(url.search, read<Pending>(PENDING));
  if (ret.kind === "none") return { kind: "none" };
  for (const k of ["code", "state", "error", "error_description", "error_uri"]) url.searchParams.delete(k);
  window.history.replaceState(null, "", url.pathname + url.search);
  write(PENDING, undefined);
  if (ret.kind === "refused") return ret;
  try {
    const tokens = fromGrant(
      await exchangeGithubCode(profile, {
        code: ret.code,
        redirectUri: ret.pending.redirectUri,
        codeVerifier: ret.pending.verifier,
      }),
    );
    hold(tokens);
    return { kind: "signed-in", tokens };
  } catch (err) {
    return { kind: "refused", why: err instanceof Error ? err.message : String(err) };
  }
}

/** The access token, refreshed first when it lapses within five minutes and a refresh token is held. */
export async function freshToken(profile: InstallProfile): Promise<string | undefined> {
  const t = heldTokens();
  if (!t) return undefined;
  if (t.refresh && t.expiresAt !== undefined && t.expiresAt - Date.now() < REFRESH_AHEAD_MS) {
    try {
      const next = fromGrant(await refreshGithubToken(profile, t.refresh));
      hold({ ...next, ...(next.refresh ? {} : { refresh: t.refresh }) });
      return next.access;
    } catch {
      return t.expiresAt > Date.now() ? t.access : undefined;
    }
  }
  return t.access;
}

/** Forgets the tokens. */
export function signOut(): void {
  hold(undefined);
  write(PENDING, undefined);
}

/** The account a login opened last time, remembered across tabs as a convenience. */
export function rememberedAccount(login: string): string | undefined {
  try {
    return window.localStorage.getItem(`${LAST_ACCOUNT}.${login}`) ?? undefined;
  } catch {
    return undefined;
  }
}

export function rememberAccount(login: string, account: string): void {
  try {
    window.localStorage.setItem(`${LAST_ACCOUNT}.${login}`, account);
  } catch {
    // A remembered account is a convenience; without storage the next sign-in chooses again.
  }
}

/** Which account to enter, or why the page must ask. */
export type Choice =
  | { kind: "enter"; account: string }
  | { kind: "choose" }
  | { kind: "none" }
  | { kind: "refused"; why: string };

/**
 * The account choice (design 03 § signing in hosted): a pinned account is
 * entered when the identity holds it and refused otherwise; one
 * membership is entered without asking; several are offered unless one
 * was remembered; none says so.
 */
export function chooseAccount(
  memberships: IdentityMembership[],
  pinned: string | undefined,
  remembered: string | undefined,
): Choice {
  if (pinned) {
    return memberships.some((m) => m.account === pinned)
      ? { kind: "enter", account: pinned }
      : {
          kind: "refused",
          why: `this interface serves account ${pinned}, and your GitHub identity is not a member of it`,
        };
  }
  if (memberships.length === 0) return { kind: "none" };
  if (memberships.length === 1) return { kind: "enter", account: memberships[0]!.account };
  const back = memberships.find((m) => m.account === remembered);
  return back ? { kind: "enter", account: back.account } : { kind: "choose" };
}

/** The site an install profile was fetched from: where a person without an account goes to make one. */
export function siteOf(profileURL: string): string | undefined {
  try {
    return new URL(profileURL).origin;
  } catch {
    return undefined;
  }
}
