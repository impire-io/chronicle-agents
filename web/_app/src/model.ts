// The pure parts of the interface: what the record says, folded in the
// browser from the store's operations — rooms and direct conversations,
// their messages, who wrote what, a turn's steps, a question and its
// answer — and the mentions a message carries. No I/O here; this is what
// the tests prove.
import type { Op } from "@impire-io/chronicle";

export type RoomKind = "room" | "direct";

export interface Room {
  path: string;
  id: string;
  title: string;
  purpose: string;
  status: "open" | "closed";
  /** A room opened for a purpose, or a direct conversation between two parties (decision 0011). */
  kind: RoomKind;
  /** A direct conversation's two parties, sorted. */
  participants: string[];
  /** The message a room was broken out from, when it was. */
  origin: string;
  createdSeq: number;
  lastSeq: number;
  lastAuthor: string;
  lastTs: string;
  messages: Message[];
}

export type MessageKind = "message" | "turn" | "system" | "question";

/** One tool call within a turn, as the turn records it (decision 0010). */
export interface Step {
  tool: string;
  input: string;
  output: string;
  ok: boolean;
  tookMs: number;
}

/** What a turn cost, as the harness reported it. */
export interface Usage {
  model: string;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  turns: number;
}

export interface Message {
  path: string;
  seq: number;
  opId: string;
  author: string;
  ts: string;
  text: string;
  kind: MessageKind;
  mentions: string[];
  replyTo: string;
  /** On a question: the answers offered, in order. */
  choices: string[];
  /** On a turn: the tool calls it took. */
  steps: Step[];
  usage?: Usage;
}

/** An agent the workspace knows: the runner's announcement, `agent/<name>` (design 01, decision 0009). */
export interface Agent {
  name: string;
  description: string;
  harness: string;
  triggers: string[];
  /** What it may use: the harness's tools granted and the toolkits, as announced (decision 0010). */
  tools: string[];
  version: string;
}

export interface Workspace {
  rooms: Map<string, Room>;
  agents: Map<string, Agent>;
  /** The sequence the fold stands at. */
  seq: number;
}

export const emptyWorkspace = (): Workspace => ({
  rooms: new Map(),
  agents: new Map(),
  seq: 0,
});

/** The @names a text mentions, in order, each once; the contract an agent's mention trigger matches. */
export function mentionsOf(text: string): string[] {
  const out: string[] = [];
  for (const m of text.matchAll(/(^|[^A-Za-z0-9_-])@([A-Za-z0-9_-]+)/g)) {
    const name = m[2];
    if (!out.includes(name)) out.push(name);
  }
  return out;
}

/** The path tokens of an op's subject within a store: CHRON.<store>.OPS.<tail> → the path. */
export function pathOf(store: string, subject: string): string {
  const prefix = `CHRON.${store}.OPS.`;
  if (!subject.startsWith(prefix)) return "";
  return subject.slice(prefix.length).split(".").join("/");
}

const parse = (payload: unknown): Record<string, unknown> => {
  if (payload instanceof Uint8Array) {
    try {
      return JSON.parse(new TextDecoder().decode(payload)) as Record<
        string,
        unknown
      >;
    } catch {
      return {};
    }
  }
  if (typeof payload === "string") {
    try {
      return JSON.parse(payload) as Record<string, unknown>;
    } catch {
      return {};
    }
  }
  return (payload ?? {}) as Record<string, unknown>;
};

const str = (v: unknown): string => (typeof v === "string" ? v : "");
const num = (v: unknown): number =>
  typeof v === "number" && Number.isFinite(v) ? v : 0;
const strings = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((s): s is string => typeof s === "string") : [];

const stepsOf = (v: unknown): Step[] => {
  if (!Array.isArray(v)) return [];
  const out: Step[] = [];
  for (const s of v) {
    if (!s || typeof s !== "object") continue;
    const o = s as Record<string, unknown>;
    out.push({
      tool: str(o.tool),
      input: str(o.input),
      output: str(o.output),
      ok: o.ok === true,
      tookMs: num(o.took_ms),
    });
  }
  return out;
};

const usageOf = (v: unknown): Usage | undefined => {
  if (!v || typeof v !== "object") return undefined;
  const o = v as Record<string, unknown>;
  return {
    model: str(o.model),
    inputTokens: num(o.input_tokens),
    outputTokens: num(o.output_tokens),
    costUsd: num(o.cost_usd),
    turns: num(o.turns),
  };
};

/**
 * Folds one operation into the workspace. Rooms are room/<id>; messages
 * are room/<id>/messages/<id>; everything else in the store is left alone.
 * Returns the room the op touched, if any.
 */
export function fold(ws: Workspace, store: string, op: Op): Room | undefined {
  ws.seq = Math.max(ws.seq, op.seq);
  const path = pathOf(store, op.subject);
  const toks = path.split("/");
  if (toks[0] === "agent" && toks.length === 2) {
    foldAgent(ws, toks[1], op);
    return undefined;
  }
  if (toks[0] !== "room" || toks.length < 2) return undefined;
  const roomPath = `room/${toks[1]}`;
  let room = ws.rooms.get(roomPath);
  if (!room) {
    room = {
      path: roomPath,
      id: toks[1],
      title: toks[1],
      purpose: "",
      status: "open",
      kind: "room",
      participants: [],
      origin: "",
      createdSeq: op.seq,
      lastSeq: op.seq,
      lastAuthor: op.author,
      lastTs: op.ts,
      messages: [],
    };
    ws.rooms.set(roomPath, room);
  }
  const data = parse(op.payload);
  if (toks.length === 2) {
    switch (op.type) {
      case "create":
      case "rename":
      case "snapshot": {
        const s =
          op.type === "snapshot"
            ? ((data.state ?? {}) as Record<string, unknown>)
            : data;
        if (str(s.title)) room.title = str(s.title);
        if (str(s.purpose)) room.purpose = str(s.purpose);
        if (s.status === "closed" || s.status === "open")
          room.status = s.status;
        if (s.kind === "direct") room.kind = "direct";
        if (strings(s.participants).length > 0)
          room.participants = [...strings(s.participants)].sort();
        if (str(s.origin)) room.origin = str(s.origin);
        break;
      }
      case "close":
        room.status = "closed";
        break;
    }
    room.lastSeq = Math.max(room.lastSeq, op.seq);
    return room;
  }
  if (toks.length === 4 && toks[2] === "messages" && op.type === "create") {
    const kind: MessageKind =
      data.kind === "turn" || data.kind === "system" || data.kind === "question"
        ? data.kind
        : "message";
    room.messages.push({
      path,
      seq: op.seq,
      opId: op.id,
      author: op.author,
      ts: op.ts,
      text: str(data.text),
      kind,
      mentions: strings(data.mentions),
      replyTo: str(data.reply_to),
      choices: strings(data.choices),
      steps: stepsOf(data.steps),
      usage: usageOf(data.usage),
    });
    room.lastSeq = op.seq;
    room.lastAuthor = op.author;
    room.lastTs = op.ts;
    return room;
  }
  return room;
}

/** The runner's announcement of an agent: create, update and a snapshot all merge into what the workspace knows. */
function foldAgent(ws: Workspace, name: string, op: Op): void {
  if (op.type !== "create" && op.type !== "update" && op.type !== "snapshot")
    return;
  const data = parse(op.payload);
  const s =
    op.type === "snapshot"
      ? ((data.state ?? {}) as Record<string, unknown>)
      : data;
  const agent = ws.agents.get(name) ?? {
    name,
    description: "",
    harness: "",
    triggers: [],
    tools: [],
    version: "",
  };
  if (str(s.description)) agent.description = str(s.description);
  if (str(s.harness)) agent.harness = str(s.harness);
  if (Array.isArray(s.triggers)) agent.triggers = strings(s.triggers);
  if (Array.isArray(s.tools)) agent.tools = strings(s.tools);
  if (str(s.version)) agent.version = str(s.version);
  ws.agents.set(name, agent);
}

/** Consecutive messages by one author of one kind, grouped under one name — a chat's reading of a transcript; a question always stands alone. */
export function groupMessages(messages: Message[]): Message[][] {
  const groups: Message[][] = [];
  for (const m of messages) {
    const last = groups[groups.length - 1];
    if (
      last &&
      m.kind !== "question" &&
      last[0].kind !== "question" &&
      last[0].author === m.author &&
      last[0].kind === m.kind
    )
      last.push(m);
    else groups.push([m]);
  }
  return groups;
}

/** The id of the room `general` — where a workspace starts (design 01 § the room). */
export const GENERAL = "general";
export const generalRoom = {
  title: "General",
  purpose: "where a workspace starts",
  status: "open" as const,
};

/** The direct conversation between two parties: one id whoever starts it (decision 0011). */
export function directId(a: string, b: string): string {
  const [x, y] = [a, b].sort();
  return `direct--${x}--${y}`;
}

/** The state a direct conversation is created with. */
export function directRoom(a: string, b: string) {
  const [x, y] = [a, b].sort();
  return {
    title: `${x} & ${y}`,
    purpose: "a direct conversation",
    status: "open" as const,
    kind: "direct" as const,
    participants: [x, y],
  };
}

/** The other party of a direct conversation, from the viewer's side. */
export function otherParty(room: Room, me: string): string {
  return room.participants.find((p) => p !== me) ?? room.participants[0] ?? "";
}

/** What a room is called from the viewer's side: the other party's name for a direct conversation, its title otherwise. */
export function titleFor(room: Room, me: string): string {
  return room.kind === "direct"
    ? otherParty(room, me) || room.title
    : room.title;
}

/** The viewer's direct conversations, open first, most recent first; one they are not a party to is not theirs to list. */
export function conversationsOf(ws: Workspace, me: string): Room[] {
  return [...ws.rooms.values()]
    .filter((r) => r.kind === "direct" && r.participants.includes(me))
    .sort((a, b) =>
      a.status !== b.status
        ? a.status === "open"
          ? -1
          : 1
        : b.lastSeq - a.lastSeq,
    );
}

/** The rooms that are not direct conversations: open first, most recent activity first. */
export function roomOrder(ws: Workspace): Room[] {
  return [...ws.rooms.values()]
    .filter((r) => r.kind !== "direct")
    .sort((a, b) => {
      if (a.status !== b.status) return a.status === "open" ? -1 : 1;
      return b.lastSeq - a.lastSeq;
    });
}

/**
 * Where the page opens: the open room or conversation of the viewer's that
 * last had a message, else nothing — the entry state, where a person
 * starts (decision 0011).
 */
export function lastActiveRoom(ws: Workspace, me = ""): Room | undefined {
  let best: Room | undefined;
  for (const r of ws.rooms.values()) {
    if (r.status !== "open" || r.messages.length === 0) continue;
    if (r.kind === "direct" && me && !r.participants.includes(me)) continue;
    if (!best || r.lastSeq > best.lastSeq) best = r;
  }
  return best;
}

/** Whether the page should make General itself: the record is read and holds no room at all. */
export function needsGeneral(ws: Workspace, read: boolean): boolean {
  return read && ws.rooms.size === 0;
}

/** The state a breakout room is created with: a room for a purpose, with where it came from. */
export function breakoutRoom(title: string, purpose: string, origin: string) {
  return {
    title,
    purpose,
    status: "open" as const,
    kind: "room" as const,
    origin,
  };
}

/** The system line a conversation gets when a room is broken out of it. */
export function breakoutLine(
  roomTitle: string,
  roomPath: string,
): { text: string; kind: "system" } {
  return { text: `continued in ${roomTitle} (${roomPath})`, kind: "system" };
}

/** The questions in a transcript that nobody has answered yet. */
export function openQuestions(messages: Message[]): Message[] {
  const answered = new Set(
    messages.filter((m) => m.replyTo).map((m) => m.replyTo),
  );
  return messages.filter((m) => m.kind === "question" && !answered.has(m.path));
}

/** The answer to a question, when one landed. */
export function answerTo(
  messages: Message[],
  question: Message,
): Message | undefined {
  return messages.find(
    (m) => m.replyTo === question.path && m.author !== question.author,
  );
}

/** One entry of the @ picker and of the conversation starters. */
export interface PickerItem {
  name: string;
  kind: "agent" | "person";
  description: string;
  online?: boolean;
}

/**
 * The picker's entries for what the person typed after `@`: the workspace's
 * agents first, with their line, then the account's people; never the
 * person themself. Agents the runner has not announced but the registry
 * knows as service accounts are offered by name, after the announced ones.
 */
export function pickerCandidates(
  query: string,
  agents: Agent[],
  services: string[],
  people: string[],
  exclude: string,
  online: (name: string) => boolean = () => false,
): PickerItem[] {
  const q = query.toLowerCase();
  const hit = (n: string) => n !== exclude && n.toLowerCase().startsWith(q);
  const announced = new Set(agents.map((a) => a.name));
  const out: PickerItem[] = [];
  for (const a of [...agents].sort((x, y) => x.name.localeCompare(y.name))) {
    if (hit(a.name))
      out.push({
        name: a.name,
        kind: "agent",
        description: a.description,
        online: online(a.name),
      });
  }
  for (const n of [...services].sort()) {
    if (!announced.has(n) && hit(n))
      out.push({ name: n, kind: "agent", description: "", online: online(n) });
  }
  for (const n of [...people].sort()) {
    if (!announced.has(n) && !services.includes(n) && hit(n))
      out.push({ name: n, kind: "person", description: "" });
  }
  return out;
}

/** The `@` the person is typing, when the text ends in one: the query after it. */
export function handleBeingTyped(text: string): string | undefined {
  const m = /(^|\s)@([A-Za-z0-9_-]*)$/.exec(text);
  return m ? m[2] : undefined;
}

/** Completes the handle being typed with the chosen name, a space after it. */
export function completeHandle(text: string, name: string): string {
  return text.replace(/@[A-Za-z0-9_-]*$/, `@${name} `);
}

/** The clock time of a message, for a hover. */
export function timeOf(ts: string): string {
  const t = Date.parse(ts);
  if (!ts || Number.isNaN(t)) return "";
  return new Date(t).toLocaleString();
}

/** A message id the browser mints: m-<12 hex>. */
export function mintId(random: () => number = Math.random): string {
  let s = "";
  for (let i = 0; i < 12; i++) s += Math.floor(random() * 16).toString(16);
  return `m-${s}`;
}

/** A room id from a title: lower-case, dashes, trimmed; falls back to a minted one. */
export function roomIdFrom(
  title: string,
  random: () => number = Math.random,
): string {
  const slug = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
  return slug || mintId(random).replace(/^m-/, "r-");
}

/** How long ago, in a word or two. */
export function age(ts: string, now: number = Date.now()): string {
  const t = Date.parse(ts);
  if (!ts || Number.isNaN(t)) return "";
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 5) return "now";
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.round(s / 60)}m`;
  if (s < 86400) return `${Math.round(s / 3600)}h`;
  return `${Math.round(s / 86400)}d`;
}

/** A cost, said briefly: $0.19, or less than a cent. */
export function costOf(usage: Usage | undefined): string {
  if (!usage || usage.costUsd <= 0) return "";
  if (usage.costUsd < 0.01) return "less than a cent";
  return `$${usage.costUsd.toFixed(2)}`;
}

/** A step, said in a line: "ran Bash: ls -la". */
export function stepLine(step: Step): string {
  const verb =
    step.tool === "Bash"
      ? "ran"
      : step.tool === "Read"
        ? "read"
        : step.tool.startsWith("mcp__")
          ? "called"
          : "used";
  const tool = step.tool.startsWith("mcp__")
    ? step.tool.slice(5).replace(/__/g, " › ")
    : step.tool;
  return step.input ? `${verb} ${tool}: ${step.input}` : `${verb} ${tool}`;
}
