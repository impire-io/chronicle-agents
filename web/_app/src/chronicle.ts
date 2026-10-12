// Connecting, per /config.json (chronicle-agents-hq design 03): the quick
// start's or a self-hosted context's credential over loopback, or — hosted
// — the install profile, a GitHub sign-in (signin.ts) and the identity
// bridge into an account the person holds.
import {
  Client,
  connectBridge,
  connectIdentity,
  fetchInstallProfile,
  type IdentityMembership,
  type InstallProfile,
} from "@impire-io/chronicle";
import { heldTokens } from "./signin";

export interface Config {
  mode: "local" | "hosted";
  servers?: string;
  nkeySeed?: string;
  creds?: string;
  author?: string;
  /** The install profile's address (hosted). */
  profile?: string;
  /** The account this interface is pinned to (hosted), when the operator pinned one. */
  account?: string;
  store: string;
  tickStore?: string;
}

export async function loadConfig(): Promise<Config> {
  const res = await fetch("/config.json", { cache: "no-store" });
  if (!res.ok)
    throw new Error(
      `the interface's configuration could not be read (${res.status})`,
    );
  return (await res.json()) as Config;
}

const NAME_KEY = "chronicle-agents.author";

export const rememberedName = (): string =>
  localStorage.getItem(NAME_KEY) ?? "";
export const rememberName = (name: string): void =>
  localStorage.setItem(NAME_KEY, name);

/** Whether the local form needs the person's name first: the credential carries none. */
export function askForName(cfg: Config): boolean {
  // A credential file names its principal; an nkey does not, so the
  // person says who they are — the quick start's nominal identity.
  return cfg.mode === "local" && !cfg.creds && !cfg.author && !rememberedName();
}

/** The local form: the context's credential over loopback. */
export async function connectLocal(
  cfg: Config,
): Promise<{ client: Client; author: string }> {
  const author = cfg.author || rememberedName();
  const client = await Client.connect({
    servers: cfg.servers ?? "",
    creds: cfg.creds || undefined,
    nkeySeed: cfg.nkeySeed || undefined,
    author: cfg.creds ? undefined : author,
    name: "chronicle-agents web",
  });
  return { client, author };
}

/** The install, hosted: its profile, fetched fresh, with what a browser's sign-in needs. */
export async function loadProfile(cfg: Config): Promise<InstallProfile> {
  const profile = await fetchInstallProfile(cfg.profile || undefined);
  if (!profile.websocket_url)
    throw new Error(
      "this install's profile names no websocket_url: a browser has nothing to dial",
    );
  if (!profile.github_token_url)
    throw new Error(
      "this install has no web-flow sign-in: its profile names no github_token_url",
    );
  return profile;
}

/** Who the signed-in identity is and which accounts it holds, from the identity plane. */
export async function identityOf(
  profile: InstallProfile,
  token: string,
): Promise<{ login: string; memberships: IdentityMembership[] }> {
  const id = await connectIdentity({
    servers: profile.websocket_url ?? "",
    sentinel: profile.sentinel,
    githubToken: token,
    name: "chronicle-agents web",
  });
  try {
    const reply = await id.memberships();
    return { login: id.login, memberships: reply.memberships };
  } finally {
    await id.close();
  }
}

/**
 * The bridge into an account. The token is read at every (re)connect, so
 * a placement that expires is re-placed with the token the page has kept
 * fresh in the meantime.
 */
export function enterAccount(
  profile: InstallProfile,
  account: string,
): Promise<Client> {
  return connectBridge({
    servers: profile.websocket_url ?? "",
    sentinel: profile.sentinel,
    githubToken: () => heldTokens()?.access ?? "",
    account,
    name: "chronicle-agents web",
  });
}
