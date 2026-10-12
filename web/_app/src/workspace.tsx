// The connected page (design 03 § the pages; decisions 0009, 0010 and
// 0011): one tail of the rooms store folded into conversations, rooms and
// transcripts; every write an operation; nothing kept but what the record
// says and the last few minutes of live signals. The construction is
// chronicle-web's: the sidebar shell, the top bar with the status as a
// word, the conversation in the middle.
import {
  InstanceExistsError,
  type Client,
  type ListMembersItem,
} from "@impire-io/chronicle";
import {
  BotIcon,
  CornerUpRightIcon,
  HashIcon,
  LogOutIcon,
  MessagesSquareIcon,
  PlusIcon,
  XIcon,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import { Composer } from "./composer";
import {
  BreakoutDialog,
  CloseDialog,
  NewRoomDialog,
  Refusal,
  StartConversationDialog,
} from "./dialogs";
import { Entry } from "./entry";
import {
  agentOfSubject,
  answerLanded,
  applySignal,
  emptyLiveRoom,
  expire,
  isOnline,
  parsePresence,
  parseRoomSignal,
  presenceSubjects,
  roomSubject,
  turnLanded,
  type LiveRoom,
} from "./live";
import {
  age,
  breakoutLine,
  breakoutRoom,
  conversationsOf,
  directId,
  directRoom,
  emptyWorkspace,
  fold,
  GENERAL,
  generalRoom,
  lastActiveRoom,
  mentionsOf,
  mintId,
  needsGeneral,
  otherParty,
  pathOf,
  pickerCandidates,
  roomIdFrom,
  roomOrder,
  titleFor,
  type Message,
  type Room,
  type Workspace as Ws,
} from "./model";
import { Transcript } from "./transcript";

const message = (err: unknown): string =>
  err instanceof Error ? err.message : String(err);

const roomFromLocation = (): string =>
  decodeURIComponent(location.pathname.replace(/^\/rooms\//, "")).replace(
    /^\/.*/,
    "",
  );

export function Workspace({
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
  const wsRef = useRef<Ws>(emptyWorkspace());
  const [, bump] = useState(0);
  const [status, setStatus] = useState<"replaying" | "live" | "ended">(
    "replaying",
  );
  const [error, setError] = useState<string>();
  const [current, setCurrent] = useState<string>(roomFromLocation);
  const [members, setMembers] = useState<ListMembersItem[]>([]);
  const [starting, setStarting] = useState(false);
  const [creating, setCreating] = useState(false);
  const [breaking, setBreaking] = useState(false);
  const [closing, setClosing] = useState(false);
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

  const liveOf = useCallback((id: string): LiveRoom => {
    let live = liveRef.current.get(id);
    if (!live) {
      live = emptyLiveRoom();
      liveRef.current.set(id, live);
    }
    return live;
  }, []);

  // One tail of the store, from the start: history feeds the fold, then it stays live.
  useEffect(() => {
    const abort = new AbortController();
    void (async () => {
      try {
        const head = await headOf(client, store);
        if (head === 0) setStatus("live");
        for await (const op of client.tail(store, "", {
          signal: abort.signal,
        })) {
          const room = fold(wsRef.current, store, op);
          if (
            room &&
            op.type === "create" &&
            op.subject.includes(".messages.")
          ) {
            const m = room.messages[room.messages.length - 1];
            if (m && m.path === pathOf(store, op.subject)) {
              const live = liveRef.current.get(room.id);
              if (live) {
                if (m.kind === "turn") turnLanded(live, m.author);
                if (m.replyTo) answerLanded(live, m.replyTo);
              }
            }
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

  // The open room's live subject: what the agents in it are doing, what was refused — while it is open.
  useEffect(() => {
    if (!current) return;
    const live = liveOf(current);
    live.lines.length = 0;
    const sub = client.connection.subscribe(roomSubject(store, current));
    void (async () => {
      for await (const m of sub) {
        const s = parseRoomSignal(m.data);
        if (!s) continue;
        applySignal(live, s, Date.now());
        bump((n) => n + 1);
      }
    })().catch(() => undefined);
    return () => sub.unsubscribe();
  }, [client, store, current, liveOf]);

  // Where the page opens, once the record is read: the URL's room, else the
  // conversation or room that last had a message, else the entry state;
  // a workspace with no room at all gets General.
  const chosen = useRef(false);
  useEffect(() => {
    if (status === "replaying" || chosen.current) return;
    const ws = wsRef.current;
    chosen.current = true;
    if (current && ws.rooms.has(`room/${current}`)) return;
    if (needsGeneral(ws, true)) {
      void client
        .create(store, `room/${GENERAL}`, "create", generalRoom)
        .catch((err) => {
          if (!(err instanceof InstanceExistsError)) setError(message(err));
        });
    }
    const last = lastActiveRoom(ws, author);
    if (last) open(last.id, true);
    else if (current) open("", true);
  }, [status, current, client, store, author, open]);

  useEffect(() => {
    const onPop = () => setCurrent(roomFromLocation());
    addEventListener("popstate", onPop);
    return () => removeEventListener("popstate", onPop);
  }, []);

  const ws = wsRef.current;
  const conversations = conversationsOf(ws, author);
  const rooms = roomOrder(ws);
  const room = current ? ws.rooms.get(`room/${current}`) : undefined;
  const agents = useMemo(
    () => [...ws.agents.values()].sort((a, b) => a.name.localeCompare(b.name)),
    [ws.agents.size, status, now],
  ); // eslint-disable-line react-hooks/exhaustive-deps -- the map is mutated in place; its size and the clock say when to re-read it
  const services = useMemo(
    () => members.filter((m) => m.kind === "service").map((m) => m.name),
    [members],
  );
  const people = useMemo(
    () => members.filter((m) => m.kind !== "service").map((m) => m.name),
    [members],
  );
  const agentNames = useMemo(
    () => new Set([...services, ...agents.map((a) => a.name)]),
    [services, agents],
  );
  const online = useCallback(
    (name: string) => isOnline(heardRef.current.get(name), now),
    [now],
  );
  const isAgent = useCallback(
    (name: string) => agentNames.has(name),
    [agentNames],
  );
  const starters = useMemo(
    () => pickerCandidates("", agents, services, people, author, online),
    [agents, services, people, author, online],
  );

  const startConversation = async (name: string) => {
    setStarting(false);
    const id = directId(author, name);
    if (!ws.rooms.has(`room/${id}`)) {
      try {
        await client.create(
          store,
          `room/${id}`,
          "create",
          directRoom(author, name),
        );
      } catch (err) {
        if (!(err instanceof InstanceExistsError)) {
          setError(message(err));
          return;
        }
      }
    }
    open(id);
  };

  const newRoom = async (title: string, purpose: string) => {
    const id = roomIdFrom(title);
    await client.create(store, `room/${id}`, "create", {
      title,
      purpose,
      status: "open",
      kind: "room",
    });
    setCreating(false);
    open(id);
  };

  const breakOut = async (title: string, purpose: string) => {
    if (!room) return;
    const id = roomIdFrom(title);
    const origin = room.messages[room.messages.length - 1]?.path ?? room.path;
    await client.create(
      store,
      `room/${id}`,
      "create",
      breakoutRoom(title, purpose, origin),
    );
    await client.create(
      store,
      `${room.path}/messages/${mintId()}`,
      "create",
      breakoutLine(title, `room/${id}`),
    );
    setBreaking(false);
    open(id);
  };

  const send = async (text: string) => {
    if (!room) return;
    await client.create(store, `${room.path}/messages/${mintId()}`, "create", {
      text,
      kind: "message",
      mentions: mentionsOf(text),
    });
  };

  const answer = async (q: Message, text: string) => {
    if (!room) return;
    await client.create(store, `${room.path}/messages/${mintId()}`, "create", {
      text,
      kind: "message",
      mentions: [q.author],
      reply_to: q.path,
    });
  };

  const title = room ? titleFor(room, author) : "";
  const other = room && room.kind === "direct" ? otherParty(room, author) : "";

  return (
    <SidebarProvider className="h-svh">
      <AppSidebar
        conversations={conversations}
        rooms={rooms}
        current={current}
        me={author}
        account={account}
        now={now}
        isAgent={isAgent}
        online={online}
        status={status}
        onOpen={open}
        onStart={() => setStarting(true)}
        onNewRoom={() => setCreating(true)}
        onSignOut={onSignOut}
      />
      <SidebarInset className="flex h-svh min-w-0 flex-col overflow-hidden">
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-1 h-4" />
          {room ? (
            <>
              <div className="min-w-0 flex-1">
                <h1 className="truncate text-sm font-semibold">
                  {room.kind === "direct" ? (
                    <span className="flex items-center gap-2">
                      {title}
                      {isAgent(other) ? (
                        <span className="flex items-center gap-1 text-xs font-normal text-muted-foreground">
                          <span
                            className={`size-1.5 rounded-full ${online(other) ? "bg-primary" : "bg-border"}`}
                            aria-hidden="true"
                          />
                          {online(other) ? "online" : "offline"}
                        </span>
                      ) : null}
                    </span>
                  ) : (
                    title
                  )}
                </h1>
                <p className="truncate text-xs text-muted-foreground">
                  {room.kind === "direct"
                    ? "a direct conversation"
                    : room.purpose || room.path}
                  {room.origin
                    ? ` · broken out from ${originLabel(ws, room.origin, author)}`
                    : ""}
                  {room.status === "closed" ? " · closed" : ""}
                </p>
              </div>
              {room.status === "open" && room.kind === "direct" ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setBreaking(true)}
                >
                  <CornerUpRightIcon />
                  <span className="hidden sm:inline">
                    Break out into a room
                  </span>
                  <span className="sm:hidden">Break out</span>
                </Button>
              ) : null}
              {room.status === "open" ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setClosing(true)}
                  aria-label={`Close ${title}`}
                >
                  <XIcon />
                  <span className="hidden sm:inline">Close</span>
                </Button>
              ) : null}
            </>
          ) : (
            <h1 className="flex-1 truncate text-sm font-semibold">
              Conversations
            </h1>
          )}
          <StatusWord status={status} />
        </header>
        {error ? (
          <div className="px-4 pt-3">
            <Refusal>{error}</Refusal>
          </div>
        ) : null}
        {room ? (
          <>
            <Transcript
              key={room.id}
              room={room}
              me={author}
              isAgent={isAgent}
              live={liveOf(room.id)}
              now={now}
              hint={
                room.kind === "direct"
                  ? isAgent(other)
                    ? `Ask ${other} something. It answers here, and shows what it did on the way.`
                    : `Say something to ${other}.`
                  : agents.length > 0
                    ? `Say something, or @mention an agent — @${agents[0].name} is here.`
                    : "Say something, or @mention an agent."
              }
              onAnswer={answer}
            />
            {room.status === "open" ? (
              <Composer
                placeholder={
                  room.kind === "direct"
                    ? `Message ${other}`
                    : `Message ${room.title}`
                }
                draft={drafts.current.get(room.id) ?? ""}
                onDraft={(t) => drafts.current.set(room.id, t)}
                onSend={send}
                agents={agents}
                services={services}
                people={people}
                me={author}
                online={online}
                autoFocusKey={room.id}
              />
            ) : (
              <p className="border-t px-4 py-3 text-sm text-muted-foreground">
                This {room.kind === "direct" ? "conversation" : "room"} is
                closed. Nobody posts in it again, and no agent answers in it.
              </p>
            )}
          </>
        ) : (
          <Entry
            starters={starters}
            reading={status === "replaying"}
            onStart={(name) => void startConversation(name)}
            onNewRoom={() => setCreating(true)}
          />
        )}
      </SidebarInset>
      <StartConversationDialog
        open={starting}
        onOpenChange={setStarting}
        starters={starters}
        onStart={(name) => void startConversation(name)}
      />
      <NewRoomDialog
        open={creating}
        onOpenChange={setCreating}
        onCreate={newRoom}
      />
      {room ? (
        <>
          <BreakoutDialog
            open={breaking}
            onOpenChange={setBreaking}
            from={other}
            onCreate={breakOut}
          />
          <CloseDialog
            open={closing}
            onOpenChange={setClosing}
            title={title}
            name={room.id}
            onConfirm={async () => {
              await client.apply(store, room.path, "close", {
                status: "closed",
              });
              setClosing(false);
            }}
          />
        </>
      ) : null}
    </SidebarProvider>
  );
}

/** Where a breakout came from, in words: the conversation with someone, or a room's title. */
function originLabel(ws: Ws, origin: string, me: string): string {
  const id = origin.split("/")[1] ?? "";
  const from = ws.rooms.get(`room/${id}`);
  if (!from) return id || origin;
  return from.kind === "direct"
    ? `the conversation with ${otherParty(from, me)}`
    : from.title;
}

/** The stream head at the start: what is at rest is history, what lands after is live. */
async function headOf(client: Client, store: string): Promise<number> {
  let head = 0;
  for await (const item of client.listInstances(store))
    head = Math.max(head, item.seq);
  return head;
}

function StatusWord({ status }: { status: "replaying" | "live" | "ended" }) {
  const word =
    status === "live"
      ? "connected"
      : status === "replaying"
        ? "reading"
        : "disconnected";
  return (
    <span
      className="flex items-center gap-1.5 text-xs text-muted-foreground"
      role="status"
    >
      <span
        className={`size-1.5 rounded-full ${status === "live" ? "bg-primary" : status === "replaying" ? "bg-border" : "bg-destructive"}`}
        aria-hidden="true"
      />
      {word}
    </span>
  );
}

function AppSidebar({
  conversations,
  rooms,
  current,
  me,
  account,
  now,
  isAgent,
  online,
  status,
  onOpen,
  onStart,
  onNewRoom,
  onSignOut,
}: {
  conversations: Room[];
  rooms: Room[];
  current: string;
  me: string;
  account?: string;
  now: number;
  isAgent: (name: string) => boolean;
  online: (name: string) => boolean;
  status: "replaying" | "live" | "ended";
  onOpen: (id: string) => void;
  onStart: () => void;
  onNewRoom: () => void;
  onSignOut?: () => void;
}) {
  const { setOpenMobile } = useSidebar();
  const go = (id: string) => {
    onOpen(id);
    setOpenMobile(false);
  };
  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              onClick={() => go("")}
              isActive={!current}
              aria-label="Start"
            >
              <div className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
                <MessagesSquareIcon className="size-4" />
              </div>
              <div className="grid flex-1 text-left leading-tight">
                <span className="truncate font-medium">chronicle-agents</span>
                <span className="truncate text-xs text-muted-foreground">
                  {account ?? "this workspace"}
                </span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Conversations</SidebarGroupLabel>
          <SidebarGroupAction onClick={onStart} title="New conversation">
            <PlusIcon />
            <span className="sr-only">New conversation</span>
          </SidebarGroupAction>
          <SidebarGroupContent>
            <SidebarMenu>
              {conversations.length === 0 ? (
                <SidebarMenuItem>
                  <SidebarMenuButton
                    onClick={onStart}
                    className="text-muted-foreground"
                  >
                    <PlusIcon />
                    <span>Start one</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ) : null}
              {conversations.map((r) => {
                const who = otherParty(r, me);
                const agent = isAgent(who);
                return (
                  <SidebarMenuItem key={r.path}>
                    <SidebarMenuButton
                      size="lg"
                      isActive={r.id === current}
                      onClick={() => go(r.id)}
                      className={r.status === "closed" ? "opacity-60" : ""}
                    >
                      <Avatar className="size-8">
                        <AvatarFallback
                          className={agent ? "bg-primary/10 text-primary" : ""}
                        >
                          {agent ? (
                            <BotIcon className="size-4" />
                          ) : (
                            who.slice(0, 1).toUpperCase() || "?"
                          )}
                        </AvatarFallback>
                      </Avatar>
                      <div className="grid flex-1 text-left leading-tight">
                        <span className="flex items-center gap-1.5 truncate">
                          <span className="truncate font-medium">
                            {who || r.title}
                          </span>
                          {agent ? (
                            <span
                              className={`size-1.5 shrink-0 rounded-full ${online(who) ? "bg-primary" : "bg-border"}`}
                              aria-hidden="true"
                            />
                          ) : null}
                        </span>
                        <span className="truncate text-xs text-muted-foreground">
                          {r.status === "closed"
                            ? "closed"
                            : r.messages.length > 0
                              ? `${r.lastAuthor === me ? "you" : r.lastAuthor} · ${age(r.lastTs, now)}`
                              : "nothing said yet"}
                        </span>
                      </div>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Rooms</SidebarGroupLabel>
          <SidebarGroupAction onClick={onNewRoom} title="New room">
            <PlusIcon />
            <span className="sr-only">New room</span>
          </SidebarGroupAction>
          <SidebarGroupContent>
            <SidebarMenu>
              {rooms.map((r) => (
                <SidebarMenuItem key={r.path}>
                  <SidebarMenuButton
                    isActive={r.id === current}
                    onClick={() => go(r.id)}
                    className={r.status === "closed" ? "opacity-60" : ""}
                  >
                    <HashIcon />
                    <span className="truncate">{r.title}</span>
                    {r.status === "closed" ? (
                      <span className="ml-auto text-xs text-muted-foreground">
                        closed
                      </span>
                    ) : null}
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <div className="flex items-center gap-2 px-2 py-1.5 text-sm">
              <Avatar className="size-6">
                <AvatarFallback>
                  {me.slice(0, 1).toUpperCase() || "?"}
                </AvatarFallback>
              </Avatar>
              <div className="grid flex-1 leading-tight">
                <span className="truncate font-medium">{me}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {status === "live"
                    ? "connected"
                    : status === "replaying"
                      ? "reading the record"
                      : "disconnected"}
                </span>
              </div>
              {onSignOut ? (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={onSignOut}
                  aria-label="Sign out"
                  title="Sign out"
                >
                  <LogOutIcon />
                </Button>
              ) : null}
            </div>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
