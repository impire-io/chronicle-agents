// Connecting, per /config.json (chronicle-agents-hq design 03): the quick
// start's or a self-hosted context's credential over loopback, or the
// hosted install profile and a GitHub token through the identity bridge.
import { Client, connectBridge, fetchInstallProfile } from "@impire-io/chronicle";

export interface Config {
  mode: "local" | "hosted";
  servers?: string;
  nkeySeed?: string;
  creds?: string;
  author?: string;
  profile?: string;
  account?: string;
  store: string;
  tickStore?: string;
}

export async function loadConfig(): Promise<Config> {
  const res = await fetch("/config.json", { cache: "no-store" });
  if (!res.ok) throw new Error(`the interface's configuration could not be read (${res.status})`);
  return (await res.json()) as Config;
}

const TOKEN_KEY = "chronicle-agents.github-token";
const NAME_KEY = "chronicle-agents.author";

export const rememberedName = (): string => localStorage.getItem(NAME_KEY) ?? "";
export const rememberName = (name: string): void => localStorage.setItem(NAME_KEY, name);
export const rememberedToken = (): string => sessionStorage.getItem(TOKEN_KEY) ?? "";
export const rememberToken = (token: string): void => sessionStorage.setItem(TOKEN_KEY, token);
export const forgetToken = (): void => sessionStorage.removeItem(TOKEN_KEY);

/** What the page needs the person to supply before it can connect. */
export type Ask = { kind: "name" } | { kind: "token"; profile: string; account: string } | undefined;

export function askFor(cfg: Config): Ask {
  if (cfg.mode === "hosted") {
    return rememberedToken() ? undefined : { kind: "token", profile: cfg.profile ?? "", account: cfg.account ?? "" };
  }
  // A credential file names its principal; an nkey does not, so the
  // person says who they are — the quick start's nominal identity.
  if (!cfg.creds && !cfg.author && !rememberedName()) return { kind: "name" };
  return undefined;
}

export async function connect(cfg: Config): Promise<{ client: Client; author: string }> {
  if (cfg.mode === "hosted") {
    const profile = await fetchInstallProfile(cfg.profile || undefined);
    const token = rememberedToken();
    if (!token) throw new Error("a GitHub token is needed to sign in to the hosted service");
    const client = await connectBridge({
      servers: profile.websocket_url ?? "",
      sentinel: profile.sentinel,
      githubToken: token,
      account: cfg.account ?? "",
      name: "chronicle-agents web",
    });
    return { client, author: "" };
  }
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
