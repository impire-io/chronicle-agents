// The rooms, as a chat (chronicle-agents-hq design 03 § the pages,
// decision 0009): the room list, the active room's transcript anchored to
// the bottom, the agents panel, the composer with its @ picker — and the
// runner's live signals, off the record, showing who is writing and what
// was refused. One tail of the store feeds the fold; every write is an
// operation; nothing is kept but what the record says and the last few
// minutes of signals.
import { InstanceExistsError, type Client, type ListMembersItem } from "@impire-io/chronicle";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  agentOfSubject,
  applySignal,
  emptyLiveRoom,
  expire,
  isOnline,
  lineSentence,
  parsePresence,
  parseRoomSignal,
  presenceSubjects,
  roomSubject,
  turnLanded,
  writingNames,
  writingSentence,
  type LiveRoom,
} from "./live";
import {
  age,
  completeHandle,
  emptyWorkspace,
  fold,
  GENERAL,
  generalRoom,
  groupMessages,
  handleBeingTyped,
  lastActiveRoom,
  mentionsOf,
  mintId,
  needsGeneral,
  pickerCandidates,
  roomIdFrom,
  roomOrder,
  timeOf,
  type Agent,
  type Message,
  type PickerItem,
  type Room,
  type Workspace,
} from "./model";

const message = (err: unknown): string => (err instanceof Error ? err.message : String(err));

const roomFromLocation = (): string => decodeURIComponent(location.pathname.replace(/^\/rooms\//, "")).replace(/^\/.*/, "");

export function Rooms({
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
  const [current, setCurrent] = useState<string>(roomFromLocation);
  const [members, setMembers] = useState<ListMembersItem[]>([]);
  const [creating, setCreating] = useState(false);
  const [panel, setPanel] = useState(false);
  const liveRef = useRef<Map<string, LiveRoom>>(new Map());
  const heardRef = useRef<Map<string, number>>(new Map());
  const drafts = useRef<Map<string, string>>(new Map());
  const [now, setNow] = useState(() => Date.now());

  const open = useCallback((id: string, replace = false) => {
    setCurrent(id);
    const url = id ? `/rooms/${encodeURIComponent(id)}` : "/";
    if (replace) history.replaceState(null, "", url);
    else history.pushState(null, "", url);
  }, []);

  // One tail of the store, from the start: history feeds the fold, then it stays live.
  useEffect(() => {
    const abort = new AbortController();
    void (async () => {
      try {
        const head = await headOf(client, store);
        if (head === 0) setStatus("live");
        for await (const op of client.tail(store, "", { signal: abort.signal })) {
          const room = fold(wsRef.current, store, op);
          if (room && op.type === "create" && op.subject.includes(".messages.")) {
            const live = liveRef.current.get(room.id);
            if (live) turnLanded(live, op.author);
          }
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

  // Presence: every agent's heartbeat in the store, off the record.
  useEffect(() => {
    const sub = client.connection.subscribe(presenceSubjects(store));
    void (async () => {
      for await (const m of sub) {
        const p = parsePresence(m.data);
        const name = p?.agent || agentOfSubject(store, m.subject);
        if (name) heardRef.current.set(name, Date.now());
      }
    })().catch(() => undefined);
    const tick = window.setInterval(() => {
      const t = Date.now();
      for (const live of liveRef.current.values()) expire(live, t);
      setNow(t);
    }, 5_000);
    return () => {
      sub.unsubscribe();
      window.clearInterval(tick);
    };
  }, [client, store]);

  // Where the page opens, once the record is read: the room that last had a message, else General — made if there is none.
  const chosen = useRef(false);
  useEffect(() => {
    if (status === "replaying" || chosen.current) return;
    const ws = wsRef.current;
    if (current && ws.rooms.has(`room/${current}`)) {
      chosen.current = true;
      return;
    }
    if (needsGeneral(ws, true)) {
      chosen.current = true;
      void client.create(store, `room/${GENERAL}`, "create", generalRoom).catch((err) => {
        if (!(err instanceof InstanceExistsError)) setError(message(err));
      });
      open(GENERAL, true);
      return;
    }
    const last = lastActiveRoom(ws);
    if (last) {
      chosen.current = true;
      open(last.id, true);
    }
  }, [status, current, client, store, open]);

  useEffect(() => {
    const onPop = () => setCurrent(roomFromLocation());
    addEventListener("popstate", onPop);
    return () => removeEventListener("popstate", onPop);
  }, []);

  const rooms = roomOrder(wsRef.current);
  const room = current ? wsRef.current.rooms.get(`room/${current}`) : undefined;
  const agents = useMemo(() => [...wsRef.current.agents.values()].sort((a, b) => a.name.localeCompare(b.name)), [wsRef.current.agents.size, status, now]);
  const services = useMemo(() => members.filter((m) => m.kind === "service").map((m) => m.name), [members]);
  const people = useMemo(() => members.filter((m) => m.kind !== "service").map((m) => m.name), [members]);
  const agentNames = useMemo(() => new Set([...services, ...agents.map((a) => a.name)]), [services, agents]);
  const online = useCallback((name: string) => isOnline(heardRef.current.get(name), now), [now]);

  const liveOf = (id: string): LiveRoom => {
    let live = liveRef.current.get(id);
    if (!live) {
      live = emptyLiveRoom();
      liveRef.current.set(id, live);
    }
    return live;
  };

  return (
    <div className={`layout ${panel ? "panel-open" : ""}`}>
      <aside className="rooms">
        <header>
          <h1>Rooms</h1>
          <button onClick={() => setCreating(true)}>New room</button>
        </header>
        <p className="status">
          {author ? (
            <>
              as <strong>{author}</strong>
              {account ? <> in {account}</> : null} ·{" "}
            </>
          ) : null}
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
        {error ? (
          <p className="refusal" role="alert">
            {error}
          </p>
        ) : null}
        <ul>
          {rooms.map((r) => (
            <li key={r.path}>
              <button className={`room ${r.id === current ? "active" : ""} ${r.status}`} onClick={() => open(r.id)}>
                <span className="title">{r.title}</span>
                <span className="meta">
                  {r.status === "closed" ? "closed · " : ""}
                  {r.messages.length > 0 ? (
                    <>
                      {r.lastAuthor} · {age(r.lastTs, now)}
                    </>
                  ) : (
                    "nothing said yet"
                  )}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </aside>
      <main className="room-view">
        {room ? (
          <RoomView
            key={room.id}
            client={client}
            store={store}
            room={room}
            author={author}
            agents={agents}
            agentNames={agentNames}
            services={services}
            people={people}
            online={online}
            live={liveOf(room.id)}
            now={now}
            draft={drafts.current.get(room.id) ?? ""}
            onDraft={(t) => drafts.current.set(room.id, t)}
            onTogglePanel={() => setPanel((p) => !p)}
            onLive={() => bump((n) => n + 1)}
          />
        ) : (
          <div className="centered">
            <p className="muted">{status === "replaying" ? "Reading the rooms…" : "Choose a room, or open one."}</p>
          </div>
        )}
      </main>
      <AgentsPanel agents={agents} services={services} people={people} online={online} onClose={() => setPanel(false)} />
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
  agentNames,
  services,
  people,
  online,
  live,
  now,
  draft,
  onDraft,
  onTogglePanel,
  onLive,
}: {
  client: Client;
  store: string;
  room: Room;
  author: string;
  agents: Agent[];
  agentNames: Set<string>;
  services: string[];
  people: string[];
  online: (name: string) => boolean;
  live: LiveRoom;
  now: number;
  draft: string;
  onDraft: (text: string) => void;
  onTogglePanel: () => void;
  onLive: () => void;
}) {
  const [text, setTextState] = useState(draft);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [closing, setClosing] = useState(false);
  const [pick, setPick] = useState(0);
  const [menu, setMenu] = useState<string>();
  const list = useRef<HTMLOListElement>(null);
  const atBottom = useRef(true);
  const textarea = useRef<HTMLTextAreaElement>(null);

  const setText = (t: string) => {
    setTextState(t);
    onDraft(t);
  };

  // The room's live subject: who is writing, what was refused — this room's, while it is open.
  useEffect(() => {
    live.lines.length = 0;
    const sub = client.connection.subscribe(roomSubject(store, room.id));
    void (async () => {
      for await (const m of sub) {
        const s = parseRoomSignal(m.data);
        if (!s) continue;
        applySignal(live, s, Date.now());
        onLive();
      }
    })().catch(() => undefined);
    return () => sub.unsubscribe();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- one subscription per room, for its life
  }, [client, store, room.id]);

  // Anchored to the bottom: follows while the person is there, stays while they read back.
  const onScroll = () => {
    const el = list.current;
    if (!el) return;
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };
  useEffect(() => {
    const el = list.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [room.messages.length, live.writing.size, live.lines.length]);
  useEffect(() => {
    const el = list.current;
    if (el) el.scrollTop = el.scrollHeight;
    atBottom.current = true;
    textarea.current?.focus();
  }, [room.id]);

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
      atBottom.current = true;
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  };

  const typed = handleBeingTyped(text);
  const candidates: PickerItem[] = useMemo(
    () => (typed === undefined ? [] : pickerCandidates(typed, agents, services, people, author, online).slice(0, 8)),
    [typed, agents, services, people, author, online],
  );
  useEffect(() => setPick(0), [typed]);
  const complete = (name: string) => {
    setText(completeHandle(text, name));
    textarea.current?.focus();
  };

  const groups = groupMessages(room.messages);
  const writing = writingSentence(writingNames(live));
  void now;

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
        <div className="actions">
          <button type="button" className="quiet agents-toggle" onClick={onTogglePanel}>
            Agents
          </button>
          {room.status === "open" ? (
            <button className="quiet" onClick={() => setClosing(true)}>
              Close room
            </button>
          ) : null}
        </div>
      </header>
      <ol className="transcript" aria-label="Transcript" ref={list} onScroll={onScroll}>
        {room.messages.length === 0 ? (
          <li className="empty">
            Nothing said here yet.{agents.length > 0 ? ` Ask an agent — @${agents[0].name} is here.` : " Say something, or @mention an agent."}
          </li>
        ) : null}
        {groups.map((g) => {
          const first = g[0];
          const isAgent = agentNames.has(first.author) || first.kind === "turn";
          return (
            <li key={first.path} className={`group ${first.kind} ${isAgent ? "agent" : ""} ${first.author === author ? "mine" : ""}`}>
              <div className="who">
                <span className="author">{first.author}</span>
                {isAgent ? <span className="badge">agent</span> : null}
                <span className="when" title={timeOf(first.ts)}>
                  {age(first.ts, now)}
                </span>
              </div>
              {g.map((m) => (
                <MessageLine key={m.path} m={m} open={menu === m.path} onMenu={() => setMenu(menu === m.path ? undefined : m.path)} />
              ))}
            </li>
          );
        })}
        {live.lines.map((l) => (
          <li key={l.id} className={`live-line ${l.state}`} role="status">
            {lineSentence(l)}
          </li>
        ))}
      </ol>
      {writing ? (
        <p className="writing" role="status" aria-live="polite">
          <span className="pulse" aria-hidden="true" /> {writing}
        </p>
      ) : null}
      {room.status === "open" ? (
        <form
          className="composer"
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          {error ? (
            <p className="refusal" role="alert">
              {error}
            </p>
          ) : null}
          {candidates.length > 0 ? (
            <ul className="picker" role="listbox" aria-label="Who to mention">
              {candidates.map((c, i) => (
                <li key={c.name}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={i === pick}
                    className={i === pick ? "active" : ""}
                    onMouseEnter={() => setPick(i)}
                    onClick={() => complete(c.name)}
                  >
                    <span className="name">
                      {c.kind === "agent" ? <span className={`presence ${c.online ? "online" : "offline"}`} aria-hidden="true" /> : null}@{c.name}
                      {c.kind === "agent" ? <span className="badge">agent</span> : null}
                    </span>
                    {c.description ? <span className="description">{c.description}</span> : null}
                  </button>
                </li>
              ))}
            </ul>
          ) : null}
          <textarea
            ref={textarea}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (candidates.length > 0) {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  setPick((p) => (p + 1) % candidates.length);
                  return;
                }
                if (e.key === "ArrowUp") {
                  e.preventDefault();
                  setPick((p) => (p - 1 + candidates.length) % candidates.length);
                  return;
                }
                if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
                  e.preventDefault();
                  complete(candidates[pick].name);
                  return;
                }
                if (e.key === "Escape") {
                  e.preventDefault();
                  setText(text + " ");
                  return;
                }
              }
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send();
              }
            }}
            placeholder={`Message ${room.title} — @ reaches an agent, Enter sends, Shift+Enter breaks a line`}
            rows={2}
          />
          <button type="submit" disabled={busy || !text.trim()}>
            Send
          </button>
        </form>
      ) : (
        <p className="composer closed muted">This room is closed. Nobody posts in it again, and no agent answers in it.</p>
      )}
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

function MessageLine({ m, open, onMenu }: { m: Message; open: boolean; onMenu: () => void }) {
  return (
    <div className={`msg ${m.kind}`}>
      <div className="text">{m.text}</div>
      <button type="button" className="more" aria-label="About this message" aria-expanded={open} onClick={onMenu}>
        ···
      </button>
      {open ? (
        <div className="about" role="menu">
          <span>
            sequence <span className="seq">#{m.seq}</span>
          </span>
          <span>{timeOf(m.ts)}</span>
          {m.replyTo ? <span>in reply to {m.replyTo.split("/").pop()}</span> : null}
          {m.mentions.length > 0 ? <span>mentions {m.mentions.map((n) => `@${n}`).join(", ")}</span> : null}
        </div>
      ) : null}
    </div>
  );
}

function AgentsPanel({
  agents,
  services,
  people,
  online,
  onClose,
}: {
  agents: Agent[];
  services: string[];
  people: string[];
  online: (name: string) => boolean;
  onClose: () => void;
}) {
  const announced = new Set(agents.map((a) => a.name));
  const unannounced = services.filter((n) => !announced.has(n)).sort();
  return (
    <aside className="agents" aria-label="Agents and people">
      <header>
        <h2>Agents</h2>
        <button type="button" className="quiet agents-toggle" onClick={onClose}>
          Close
        </button>
      </header>
      {agents.length === 0 && unannounced.length === 0 ? (
        <p className="empty">No agent here yet. One appears when it runs: chronicle-agents run wilson.yaml</p>
      ) : null}
      <ul>
        {agents.map((a) => (
          <li key={a.name} className="agent" title={a.triggers.length > 0 ? `reacts to: ${a.triggers.join("; ")}` : undefined}>
            <span className="name">
              <span className={`presence ${online(a.name) ? "online" : "offline"}`} aria-hidden="true" />@{a.name}
              <span className="state">{online(a.name) ? "online" : "offline"}</span>
            </span>
            {a.description ? <span className="description">{a.description}</span> : <span className="description muted">no description</span>}
          </li>
        ))}
        {unannounced.map((n) => (
          <li key={n} className="agent" title="a service account the registry knows; no runner has announced it">
            <span className="name">
              <span className={`presence ${online(n) ? "online" : "offline"}`} aria-hidden="true" />@{n}
              <span className="state">{online(n) ? "online" : "offline"}</span>
            </span>
            <span className="description muted">not announced</span>
          </li>
        ))}
      </ul>
      <h2>People</h2>
      <ul>
        {people
          .slice()
          .sort()
          .map((n) => (
            <li key={n} className="person">
              <span className="name">@{n}</span>
            </li>
          ))}
      </ul>
    </aside>
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
          Purpose (optional)
          <input value={purpose} onChange={(e) => setPurpose(e.target.value)} placeholder="the finance week close" />
        </label>
        {error ? (
          <p className="refusal" role="alert">
            {error}
          </p>
        ) : null}
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
