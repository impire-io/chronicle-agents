// Live signals, off the record (chronicle-agents-hq design 02 § live
// signals, decision 0009; chronicle-hq decision 0055): what the runner says
// on CHRON.LIVE.<store>. — a trigger picked up, the harness thinking, a
// turn done, a refusal with its rule, a failure — and each agent's
// presence. Nothing here is stored; this module keeps the page's view of
// the last few minutes and forgets it on its own. The pure parts, tested.

export type SignalState = "picked-up" | "thinking" | "done" | "refused" | "failed";

/** One moment of a turn, as the runner said it on the room's live subject. */
export interface RoomSignal {
  agent: string;
  state: SignalState;
  trigger: string;
  at: string;
  reason: string;
}

/** An agent's presence, as the runner says it on its own live subject. */
export interface PresenceSignal {
  agent: string;
  state: string;
  since: string;
  version: string;
}

/** The room's live subject: CHRON.LIVE.<store>.room.<room-id>. */
export const roomSubject = (store: string, roomId: string): string => `CHRON.LIVE.${store}.room.${roomId}`;

/** Every agent's presence in the store: CHRON.LIVE.<store>.agent.*. */
export const presenceSubjects = (store: string): string => `CHRON.LIVE.${store}.agent.*`;

/** The agent a presence subject names, or "" when the subject is not one. */
export function agentOfSubject(store: string, subject: string): string {
  const prefix = `CHRON.LIVE.${store}.agent.`;
  return subject.startsWith(prefix) ? subject.slice(prefix.length) : "";
}

const decode = (data: Uint8Array | string): Record<string, unknown> | undefined => {
  try {
    const text = typeof data === "string" ? data : new TextDecoder().decode(data);
    const v = JSON.parse(text) as unknown;
    return v && typeof v === "object" ? (v as Record<string, unknown>) : undefined;
  } catch {
    return undefined;
  }
};

const str = (v: unknown): string => (typeof v === "string" ? v : "");

const states: SignalState[] = ["picked-up", "thinking", "done", "refused", "failed"];

/** A room signal from the wire, or nothing when it is not one. */
export function parseRoomSignal(data: Uint8Array | string): RoomSignal | undefined {
  const v = decode(data);
  if (!v) return undefined;
  const agent = str(v.agent);
  const state = str(v.state) as SignalState;
  if (!agent || !states.includes(state)) return undefined;
  return { agent, state, trigger: str(v.trigger), at: str(v.at), reason: str(v.reason) };
}

/** A presence signal from the wire, or nothing when it is not one. */
export function parsePresence(data: Uint8Array | string): PresenceSignal | undefined {
  const v = decode(data);
  if (!v) return undefined;
  const agent = str(v.agent);
  if (!agent) return undefined;
  return { agent, state: str(v.state) || "online", since: str(v.since), version: str(v.version) };
}

/** A line in the transcript that is plainly not a message: a refusal or a failure, with its reason. */
export interface LiveLine {
  id: string;
  agent: string;
  state: "refused" | "failed";
  reason: string;
  at: number;
}

/** What the page knows of a room's last few minutes. */
export interface LiveRoom {
  /** Agents writing now: when they picked the trigger up, and which. */
  writing: Map<string, { since: number; trigger: string }>;
  lines: LiveLine[];
}

export const emptyLiveRoom = (): LiveRoom => ({ writing: new Map(), lines: [] });

/** An agent that said picked-up and never said done is forgotten after this long — the harness's deadline, generously. */
export const WRITING_TTL_MS = 5 * 60_000;

/** A presence older than this is offline. */
export const OFFLINE_AFTER_MS = 30_000;

let lineCounter = 0;

/** Folds one room signal into the room's live view. */
export function applySignal(live: LiveRoom, s: RoomSignal, now: number): void {
  switch (s.state) {
    case "picked-up":
    case "thinking": {
      const w = live.writing.get(s.agent);
      if (w && s.state === "thinking") {
        w.since = Math.max(w.since, now - WRITING_TTL_MS / 2); // a heartbeat keeps the line alive without moving its start
        return;
      }
      live.writing.set(s.agent, { since: now, trigger: s.trigger });
      return;
    }
    case "done":
      live.writing.delete(s.agent);
      return;
    case "refused":
    case "failed":
      live.writing.delete(s.agent);
      live.lines.push({ id: `l-${++lineCounter}`, agent: s.agent, state: s.state, reason: s.reason, at: now });
      return;
  }
}

/** A turn from the agent landed in the record: it is no longer writing, whatever the signals said. */
export function turnLanded(live: LiveRoom, agent: string): void {
  live.writing.delete(agent);
}

/** Forgets what has gone stale. */
export function expire(live: LiveRoom, now: number): void {
  for (const [agent, w] of live.writing) {
    if (now - w.since > WRITING_TTL_MS) live.writing.delete(agent);
  }
}

/** Who is writing, by name, in the order they started. */
export function writingNames(live: LiveRoom): string[] {
  return [...live.writing.entries()].sort((a, b) => a[1].since - b[1].since).map(([n]) => n);
}

/** The sentence under the transcript, or "" when nobody is writing. */
export function writingSentence(names: string[]): string {
  if (names.length === 0) return "";
  if (names.length === 1) return `${names[0]} is writing…`;
  if (names.length === 2) return `${names[0]} and ${names[1]} are writing…`;
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]} are writing…`;
}

/** The words a live line shows for a refusal or a failure. */
export function lineSentence(line: LiveLine): string {
  const why = line.reason ? `: ${line.reason}` : "";
  return line.state === "refused" ? `${line.agent} did not answer${why}` : `${line.agent} could not answer${why}`;
}

/** Whether an agent whose presence was last heard then is online now. */
export function isOnline(lastHeard: number | undefined, now: number): boolean {
  return lastHeard !== undefined && now - lastHeard <= OFFLINE_AFTER_MS;
}
