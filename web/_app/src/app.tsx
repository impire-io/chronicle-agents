// The rooms interface (chronicle-agents-hq design 03): a chronicle client
// in a browser. One tail of the rooms store, folded into rooms and
// transcripts; every write an operation; no server, no second record.
// Hosted, the page signs in through GitHub's web flow and enters an
// account the identity holds (§ signing in hosted, decision 0007).
import type { Client, IdentityMembership, InstallProfile } from "@impire-io/chronicle";
import { useCallback, useEffect, useRef, useState } from "react";
import { askForName, connectLocal, enterAccount, identityOf, loadConfig, loadProfile, rememberName, type Config } from "./chronicle";
import { Rooms } from "./rooms";
import {
  chooseAccount,
  completeSignIn,
  freshToken,
  rememberAccount,
  rememberedAccount,
  signOut as forgetTokens,
  siteOf,
  startSignIn,
} from "./signin";

type Phase =
  | { kind: "loading" }
  | { kind: "ask-name"; cfg: Config }
  | { kind: "connecting"; cfg: Config; note: string }
  | { kind: "signed-out"; cfg: Config; profile: InstallProfile; refusal?: string }
  | { kind: "no-account"; cfg: Config; profile: InstallProfile; login: string }
  | { kind: "choosing"; cfg: Config; profile: InstallProfile; login: string; memberships: IdentityMembership[]; refusal?: string }
  | { kind: "connected"; cfg: Config; client: Client; author: string; account?: string; hosted: boolean }
  | { kind: "failed"; reason: string; cfg?: Config };

const message = (err: unknown): string => (err instanceof Error ? err.message : String(err));

export function App() {
  const [phase, setPhase] = useState<Phase>({ kind: "loading" });
  const clientRef = useRef<Client | null>(null);

  /** The local form: the context's credential, the person's name when the credential carries none. */
  const startLocal = useCallback(async (cfg: Config) => {
    if (askForName(cfg)) {
      setPhase({ kind: "ask-name", cfg });
      return;
    }
    setPhase({ kind: "connecting", cfg, note: "Connecting to chronicle…" });
    try {
      const { client, author } = await connectLocal(cfg);
      clientRef.current = client;
      setPhase({ kind: "connected", cfg, client, author, hosted: false });
    } catch (err) {
      setPhase({ kind: "failed", reason: message(err), cfg });
    }
  }, []);

  /** Hosted: into the chosen account, through the bridge. */
  const enter = useCallback(
    async (cfg: Config, profile: InstallProfile, login: string, memberships: IdentityMembership[], account: string) => {
      setPhase({ kind: "connecting", cfg, note: `Opening ${account}…` });
      try {
        await clientRef.current?.close();
        const client = await enterAccount(profile, account);
        clientRef.current = client;
        rememberAccount(login, account);
        setPhase({ kind: "connected", cfg, client, author: client.author, account, hosted: true });
      } catch (err) {
        setPhase({ kind: "choosing", cfg, profile, login, memberships, refusal: message(err) });
      }
    },
    [],
  );

  /** Hosted, signed in: which accounts the identity holds, and where to go. */
  const resolve = useCallback(
    async (cfg: Config, profile: InstallProfile) => {
      const token = await freshToken(profile);
      if (!token) {
        setPhase({ kind: "signed-out", cfg, profile });
        return;
      }
      setPhase({ kind: "connecting", cfg, note: "Checking which accounts you hold…" });
      let login: string;
      let memberships: IdentityMembership[];
      try {
        ({ login, memberships } = await identityOf(profile, token));
      } catch (err) {
        forgetTokens();
        setPhase({ kind: "signed-out", cfg, profile, refusal: `Sign-in did not complete: ${message(err)}` });
        return;
      }
      const choice = chooseAccount(memberships, cfg.account, rememberedAccount(login));
      switch (choice.kind) {
        case "enter":
          await enter(cfg, profile, login, memberships, choice.account);
          return;
        case "choose":
          setPhase({ kind: "choosing", cfg, profile, login, memberships });
          return;
        case "none":
          setPhase({ kind: "no-account", cfg, profile, login });
          return;
        case "refused":
          forgetTokens();
          setPhase({ kind: "signed-out", cfg, profile, refusal: choice.why });
      }
    },
    [enter],
  );

  /** Hosted: the profile, then GitHub's return if this is one, then the accounts. */
  const startHosted = useCallback(
    async (cfg: Config) => {
      setPhase({ kind: "connecting", cfg, note: "Reading the install…" });
      let profile: InstallProfile;
      try {
        profile = await loadProfile(cfg);
      } catch (err) {
        setPhase({ kind: "failed", reason: message(err), cfg });
        return;
      }
      const ret = await completeSignIn(profile);
      if (ret.kind === "refused") {
        setPhase({ kind: "signed-out", cfg, profile, refusal: ret.why });
        return;
      }
      await resolve(cfg, profile);
    },
    [resolve],
  );

  const start = useCallback(
    (cfg: Config) => (cfg.mode === "hosted" ? startHosted(cfg) : startLocal(cfg)),
    [startHosted, startLocal],
  );

  useEffect(() => {
    void loadConfig().then(start, (err) => setPhase({ kind: "failed", reason: message(err) }));
    // Keep the GitHub token fresh, so a re-placement through the bridge carries a live one.
    const keep = window.setInterval(() => {
      setPhase((p) => {
        if (p.kind === "connected" && p.hosted && p.cfg.profile !== undefined) {
          void loadProfile(p.cfg).then(freshToken, () => undefined);
        }
        return p;
      });
    }, 60_000);
    return () => {
      window.clearInterval(keep);
      void clientRef.current?.close();
    };
  }, [start]);

  const signOut = () => {
    forgetTokens();
    void clientRef.current?.close();
    clientRef.current = null;
    if ("cfg" in phase && phase.cfg) void start(phase.cfg);
  };

  switch (phase.kind) {
    case "loading":
      return <Centered>Loading…</Centered>;
    case "connecting":
      return <Centered>{phase.note}</Centered>;
    case "failed":
      return (
        <Centered>
          <p className="refusal" role="alert">
            {phase.reason}
          </p>
          {phase.cfg ? <button onClick={() => void start(phase.cfg!)}>Try again</button> : null}
        </Centered>
      );
    case "ask-name":
      return <AskName cfg={phase.cfg} onDone={() => void startLocal(phase.cfg)} />;
    case "signed-out":
      return (
        <Centered>
          <div className="card">
            <h1>Rooms</h1>
            <p>
              Humans and agents, collaborating in the same place. Sign in with GitHub to open the rooms of an account you
              hold{phase.cfg.account ? <> — this interface serves <strong>{phase.cfg.account}</strong></> : null}.
            </p>
            {phase.refusal ? (
              <p className="refusal" role="alert">
                {phase.refusal}
              </p>
            ) : null}
            <div className="actions">
              <button onClick={() => void startSignIn(phase.profile)}>Sign in with GitHub</button>
            </div>
          </div>
        </Centered>
      );
    case "no-account":
      return (
        <Centered>
          <div className="card">
            <h1>No account yet</h1>
            <p>
              <strong>{phase.login}</strong> holds no account on this chronicle. An account is created at the first
              sign-in on the site, or by an invitation from an account's admin.
            </p>
            <div className="actions">
              <button type="button" className="quiet" onClick={signOut}>
                Sign out
              </button>
              {siteOf(phase.cfg.profile ?? "") ? (
                <a className="button" href={`${siteOf(phase.cfg.profile ?? "")}/console/`}>
                  Open the site
                </a>
              ) : null}
            </div>
          </div>
        </Centered>
      );
    case "choosing":
      return (
        <Centered>
          <div className="card">
            <h1>Choose an account</h1>
            <p className="muted">{phase.login} is a member of several. Open one.</p>
            {phase.refusal ? (
              <p className="refusal" role="alert">
                {phase.refusal}
              </p>
            ) : null}
            <ul className="accounts">
              {phase.memberships.map((m) => (
                <li key={m.account}>
                  <button onClick={() => void enter(phase.cfg, phase.profile, phase.login, phase.memberships, m.account)}>
                    <span className="title">{m.account}</span>
                    <span className="meta">{m.role}</span>
                  </button>
                </li>
              ))}
            </ul>
            <div className="actions">
              <button type="button" className="quiet" onClick={signOut}>
                Sign out
              </button>
            </div>
          </div>
        </Centered>
      );
    case "connected":
      return (
        <Rooms
          client={phase.client}
          store={phase.cfg.store}
          author={phase.author}
          account={phase.account}
          onSignOut={phase.hosted ? signOut : undefined}
        />
      );
  }
}

function Centered({ children }: { children: React.ReactNode }) {
  return <main className="centered">{children}</main>;
}

function AskName({ cfg, onDone }: { cfg: Config; onDone: () => void }) {
  const [value, setValue] = useState("");
  return (
    <Centered>
      <form
        className="card"
        onSubmit={(e) => {
          e.preventDefault();
          if (!value.trim()) return;
          rememberName(value.trim());
          onDone();
        }}
      >
        <h1>Rooms</h1>
        <p>
          This chronicle ({cfg.servers}) names no one: the quick start has one user. Say who you are — the name your
          messages carry.
        </p>
        <label>
          Your name
          <input autoFocus value={value} onChange={(e) => setValue(e.target.value)} placeholder="daan" />
        </label>
        <button type="submit">Continue</button>
      </form>
    </Centered>
  );
}
