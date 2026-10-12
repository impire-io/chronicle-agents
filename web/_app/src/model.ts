// The pure parts of the rooms interface: what the record says, folded in
// the browser from the store's operations — rooms, their messages, who
// wrote what — and the mentions a message carries. No I/O here; this is
// what the tests prove.
import type { Op } from "@impire-io/chronicle";

export interface Room {
  path: string;
  id: string;
  title: string;
  purpose: string;
  status: "open" | "closed";
  createdSeq: number;
  lastSeq: number;
  lastAuthor: string;
  lastTs: string;
  messages: Message[];
}

export interface Message {
  path: string;
  seq: number;
  opId: string;
  author: string;
  ts: string;
  text: string;
  kind: "message" | "turn" | "system";
  mentions: string[];
  replyTo: string;
}

/** An agent the workspace knows: the runner's announcement, `agent/<name>` (design 01, decision 0009). */
export interface Agent {
  name: string;
  description: string;
  harness: string;
  triggers: string[];
  version: string;
}

export interface Workspace {
  rooms: Map<string, Room>;
  agents: Map<string, Agent>;
  /** The sequence the fold stands at. */
  seq: number;
}

export const emptyWorkspace = (): Workspace => ({ rooms: new Map(), agents: new Map(), seq: 0 });

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
      return JSON.parse(new TextDecoder().decode(payload)) as Record<string, unknown>;
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
        const s = op.type === "snapshot" ? ((data.state ?? {}) as Record<string, unknown>) : data;
        if (str(s.title)) room.title = str(s.title);
        if (str(s.purpose)) room.purpose = str(s.purpose);
        if (s.status === "closed" || s.status === "open") room.status = s.status;
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
    const kind = data.kind === "turn" || data.kind === "system" ? data.kind : "message";
    const mentions = Array.isArray(data.mentions) ? data.mentions.filter((m): m is string => typeof m === "string") : [];
    room.messages.push({
      path,
      seq: op.seq,
      opId: op.id,
      author: op.author,
      ts: op.ts,
      text: str(data.text),
      kind,
      mentions,
      replyTo: str(data.reply_to),
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
  if (op.type !== "create" && op.type !== "update" && op.type !== "snapshot") return;
  const data = parse(op.payload);
  const s = op.type === "snapshot" ? ((data.state ?? {}) as Record<string, unknown>) : data;
  const agent = ws.agents.get(name) ?? { name, description: "", harness: "", triggers: [], version: "" };
  if (str(s.description)) agent.description = str(s.description);
  if (str(s.harness)) agent.harness = str(s.harness);
  if (Array.isArray(s.triggers)) agent.triggers = s.triggers.filter((t): t is string => typeof t === "string");
  if (str(s.version)) agent.version = str(s.version);
  ws.agents.set(name, agent);
}

/** Consecutive messages by one author, grouped under one name — a chat's reading of a transcript. */
export function groupMessages(messages: Message[]): Message[][] {
  const groups: Message[][] = [];
  for (const m of messages) {
    const last = groups[groups.length - 1];
    if (last && last[0].author === m.author && last[0].kind === m.kind) last.push(m);
    else groups.push([m]);
  }
  return groups;
}

/** The id of the room `general` — where a workspace starts (design 01 § the room). */
export const GENERAL = "general";
export const generalRoom = { title: "General", purpose: "where a workspace starts", status: "open" as const };

/**
 * Where the page opens: the open room that last had a message, else
 * General, else the first open room, else none at all.
 */
export function lastActiveRoom(ws: Workspace): Room | undefined {
  let best: Room | undefined;
  for (const r of ws.rooms.values()) {
    if (r.status !== "open" || r.messages.length === 0) continue;
    if (!best || r.lastSeq > best.lastSeq) best = r;
  }
  if (best) return best;
  const general = ws.rooms.get(`room/${GENERAL}`);
  if (general && general.status === "open") return general;
  return roomOrder(ws).find((r) => r.status === "open");
}

/** Whether the page should make General itself: the record is read and holds no room at all. */
export function needsGeneral(ws: Workspace, read: boolean): boolean {
  return read && ws.rooms.size === 0;
}

/** One entry of the @ picker. */
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
    if (hit(a.name)) out.push({ name: a.name, kind: "agent", description: a.description, online: online(a.name) });
  }
  for (const n of [...services].sort()) {
    if (!announced.has(n) && hit(n)) out.push({ name: n, kind: "agent", description: "", online: online(n) });
  }
  for (const n of [...people].sort()) {
    if (!announced.has(n) && !services.includes(n) && hit(n)) out.push({ name: n, kind: "person", description: "" });
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

/** Rooms in the order the list shows them: open first, most recent activity first. */
export function roomOrder(ws: Workspace): Room[] {
  return [...ws.rooms.values()].sort((a, b) => {
    if (a.status !== b.status) return a.status === "open" ? -1 : 1;
    return b.lastSeq - a.lastSeq;
  });
}

/** A message id the browser mints: m-<12 hex>. */
export function mintId(random: () => number = Math.random): string {
  let s = "";
  for (let i = 0; i < 12; i++) s += Math.floor(random() * 16).toString(16);
  return `m-${s}`;
}

/** A room id from a title: lower-case, dashes, trimmed; falls back to a minted one. */
export function roomIdFrom(title: string, random: () => number = Math.random): string {
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
