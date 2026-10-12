// The rooms interface (chronicle-agents-hq design 03): a chronicle client
// in a browser. One tail of the rooms store, folded into rooms and
// transcripts; every write an operation; no server, no second record.
// Hosted, the page signs in through GitHub's web flow and enters an
// account the identity holds (§ signing in hosted, decision 0007).
import type { Client, IdentityMembership, InstallProfile, ListMembersItem } from "@impire-io/chronicle";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { askForName, connectLocal, enterAccount, identityOf, loadConfig, loadProfile, rememberName, type Config } from "./chronicle";
import { age, emptyWorkspace, fold, mentionsOf, mintId, roomIdFrom, roomOrder, type Room, type Workspace } from "./model";
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

function Rooms({
  client,
  store,
  author,
  account,
  onSignOut,
}: {
  client: Client;
  store: string;
  author: string;
  account?: string;
  onSignOut?: () => void;
}) {
  const wsRef = useRef<Workspace>(emptyWorkspace());
  const [, bump] = useState(0);
  const [status, setStatus] = useState<"replaying" | "live" | "ended">("replaying");
  const [error, setError] = useState<string>();
  const [current, setCurrent] = useState<string>(() => decodeURIComponent(location.pathname.replace(/^\/rooms\//, "")).replace(/^\/.*/, ""));
  const [members, setMembers] = useState<ListMembersItem[]>([]);
  const [creating, setCreating] = useState(false);

  // One tail of the store, from the start: history feeds the fold, then it stays live.
  useEffect(() => {
    const abort = new AbortController();
    void (async () => {
      try {
        const head = await headOf(client, store);
        for await (const op of client.tail(store, "", { signal: abort.signal })) {
          fold(wsRef.current, store, op);
          if (op.seq >= head) setStatus("live");
          bump((n) => n + 1);
        }
        setStatus("ended");
      } catch (err) {
        if (!abort.signal.aborted) setError(message(err));
      }
    })();
    void (async () => {
      const out: ListMembersItem[] = [];
      try {
        for await (const m of client.listMembers()) out.push(m);
        setMembers(out);
      } catch {
        // the registry is a nicety here: names to offer, agents to mark
      }
    })();
    return () => abort.abort();
  }, [client, store]);

  const rooms = roomOrder(wsRef.current);
  const room = current ? wsRef.current.rooms.get(`room/${current}`) : undefined;
  const agents = useMemo(() => new Set(members.filter((m) => m.kind === "service").map((m) => m.name)), [members]);
  const names = useMemo(() => members.map((m) => m.name).sort(), [members]);

  const open = (id: string) => {
    setCurrent(id);
    history.pushState(null, "", id ? `/rooms/${encodeURIComponent(id)}` : "/");
  };
  useEffect(() => {
    const onPop = () => setCurrent(decodeURIComponent(location.pathname.replace(/^\/rooms\//, "")).replace(/^\/.*/, ""));
    addEventListener("popstate", onPop);
    return () => removeEventListener("popstate", onPop);
  }, []);

  return (
    <div className="layout">
      <aside className="rooms">
        <header>
          <h1>Rooms</h1>
          <button onClick={() => setCreating(true)}>New room</button>
        </header>
        <p className="status">
          {author ? <>as <strong>{author}</strong>{account ? <> in {account}</> : null} · </> : null}
          <span className={`dot ${status}`} aria-hidden="true" /> {status}
          {onSignOut ? (
            <>
              {" "}
              ·{" "}
              <button type="button" className="link" onClick={onSignOut}>
                Sign out
              </button>
            </>
          ) : null}
        </p>
        {error ? <p className="refusal" role="alert">{error}</p> : null}
        {rooms.length === 0 && status !== "replaying" ? <p className="empty">No room yet. Open one for a purpose.</p> : null}
        <ul>
          {rooms.map((r) => (
            <li key={r.path}>
              <button className={`room ${r.id === current ? "active" : ""} ${r.status}`} onClick={() => open(r.id)}>
                <span className="title">{r.title}</span>
                <span className="meta">
                  {r.status === "closed" ? "closed · " : ""}
                  {r.lastAuthor} · {age(r.lastTs)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </aside>
      <main className="room-view">
        {room ? (
          <RoomView client={client} store={store} room={room} author={author} agents={agents} names={names} />
        ) : (
          <div className="centered">
            <p>Choose a room, or open one.</p>
          </div>
        )}
      </main>
      {creating ? (
        <NewRoom
          onCancel={() => setCreating(false)}
          onCreate={async (title, purpose) => {
            const id = roomIdFrom(title);
            await client.create(store, `room/${id}`, "create", { title, purpose, status: "open" });
            setCreating(false);
            open(id);
          }}
        />
      ) : null}
    </div>
  );
}

/** The stream head at the start: what is at rest is history, what lands after is live. */
async function headOf(client: Client, store: string): Promise<number> {
  let head = 0;
  for await (const item of client.listInstances(store)) head = Math.max(head, item.seq);
  return head;
}

function RoomView({
  client,
  store,
  room,
  author,
  agents,
  names,
}: {
  client: Client;
  store: string;
  room: Room;
  author: string;
  agents: Set<string>;
  names: string[];
}) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [closing, setClosing] = useState(false);
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
  }, [room.messages.length]);

  const send = async () => {
    const t = text.trim();
    if (!t || busy) return;
    setBusy(true);
    setError(undefined);
    try {
      await client.create(store, `${room.path}/messages/${mintId()}`, "create", {
        text: t,
        kind: "message",
        mentions: mentionsOf(t),
      });
      setText("");
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  };

  const suggestions = useMemo(() => {
    const m = /(^|\s)@([A-Za-z0-9_-]*)$/.exec(text);
    if (!m) return [];
    const prefix = m[2].toLowerCase();
    return names.filter((n) => n.toLowerCase().startsWith(prefix) && n !== author).slice(0, 6);
  }, [text, names, author]);

  return (
    <>
      <header className="room-header">
        <div>
          <h2>{room.title}</h2>
          <p className="muted">
            {room.purpose || room.path} · {room.status}
            {room.status === "closed" ? " — this room is closed; it is read, not written" : ""}
          </p>
        </div>
        {room.status === "open" ? (
          <button className="quiet" onClick={() => setClosing(true)}>
            Close room
          </button>
        ) : null}
      </header>
      <ol className="transcript" aria-label="Transcript">
        {room.messages.map((m) => (
          <li key={m.path} className={`msg ${m.kind} ${agents.has(m.author) ? "agent" : ""} ${m.author === author ? "mine" : ""}`}>
            <div className="who">
              <span className="author">{m.author}</span>
              {agents.has(m.author) || m.kind === "turn" ? <span className="badge">agent</span> : null}
              <span className="seq" title="the sequence in the store's history">
                #{m.seq}
              </span>
              <span className="when">{age(m.ts)}</span>
            </div>
            <div className="text">{m.text}</div>
          </li>
        ))}
        <div ref={end} />
      </ol>
      {room.status === "open" ? (
        <form
          className="composer"
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          {error ? <p className="refusal" role="alert">{error}</p> : null}
          {suggestions.length > 0 ? (
            <div className="suggestions" role="listbox">
              {suggestions.map((n) => (
                <button
                  key={n}
                  type="button"
                  role="option"
                  aria-selected={false}
                  onClick={() => setText(text.replace(/@[A-Za-z0-9_-]*$/, `@${n} `))}
                >
                  @{n}
                  {agents.has(n) ? <span className="badge">agent</span> : null}
                </button>
              ))}
            </div>
          ) : null}
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send();
              }
            }}
            placeholder={`Message ${room.title} — @name reaches an agent`}
            rows={2}
          />
          <button type="submit" disabled={busy || !text.trim()}>
            Send
          </button>
        </form>
      ) : null}
      {closing ? (
        <Confirm
          what={`Close ${room.title}? Its messages stay, as the record they are; nobody posts in it again, and no agent answers in it.`}
          name={room.id}
          onCancel={() => setClosing(false)}
          onConfirm={async () => {
            try {
              await client.apply(store, room.path, "close", { status: "closed" });
            } catch (err) {
              setError(message(err));
            }
            setClosing(false);
          }}
        />
      ) : null}
    </>
  );
}

function NewRoom({ onCancel, onCreate }: { onCancel: () => void; onCreate: (title: string, purpose: string) => Promise<void> }) {
  const [title, setTitle] = useState("");
  const [purpose, setPurpose] = useState("");
  const [error, setError] = useState<string>();
  return (
    <div className="dialog-backdrop" onClick={onCancel}>
      <form
        className="card dialog"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          onCreate(title.trim(), purpose.trim()).catch((err) => setError(message(err)));
        }}
      >
        <h2>New room</h2>
        <p className="muted">A room is opened for a purpose and closed when it is served.</p>
        <label>
          Title
          <input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Week 41 close" />
        </label>
        <label>
          Purpose
          <input value={purpose} onChange={(e) => setPurpose(e.target.value)} placeholder="the finance week close" />
        </label>
        {error ? <p className="refusal" role="alert">{error}</p> : null}
        <div className="actions">
          <button type="button" className="quiet" onClick={onCancel}>
            Cancel
          </button>
          <button type="submit" disabled={!title.trim()}>
            Open room
          </button>
        </div>
      </form>
    </div>
  );
}

/** The irreversible act: the consequence named, the room's name typed to confirm. */
function Confirm({ what, name, onCancel, onConfirm }: { what: string; name: string; onCancel: () => void; onConfirm: () => Promise<void> }) {
  const [typed, setTyped] = useState("");
  return (
    <div className="dialog-backdrop" onClick={onCancel}>
      <form
        className="card dialog"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          if (typed === name) void onConfirm();
        }}
      >
        <h2>Close room</h2>
        <p>{what}</p>
        <label>
          Type <code>{name}</code> to confirm
          <input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} />
        </label>
        <div className="actions">
          <button type="button" className="quiet" onClick={onCancel}>
            Cancel
          </button>
          <button type="submit" className="danger" disabled={typed !== name}>
            Close room
          </button>
        </div>
      </form>
    </div>
  );
}
