import type { Op } from "@impire-io/chronicle";
import { describe, expect, it } from "vitest";
import {
  age,
  answerTo,
  breakoutLine,
  breakoutRoom,
  completeHandle,
  conversationsOf,
  costOf,
  directId,
  directRoom,
  openQuestions,
  otherParty,
  stepLine,
  titleFor,
  emptyWorkspace,
  fold,
  GENERAL,
  groupMessages,
  handleBeingTyped,
  lastActiveRoom,
  mentionsOf,
  mintId,
  needsGeneral,
  pathOf,
  pickerCandidates,
  roomIdFrom,
  roomOrder,
} from "./model";

const op = (
  seq: number,
  path: string,
  type: string,
  author: string,
  payload: unknown,
  id = `op-${seq}`,
): Op => ({
  id,
  type,
  author,
  parents: [],
  ts: new Date(1_700_000_000_000 + seq * 1000).toISOString(),
  version: "1",
  subject: `CHRON.rooms.OPS.${path.split("/").join(".")}`,
  seq,
  payload: new TextEncoder().encode(JSON.stringify(payload)),
});

describe("mentions", () => {
  it("reads the @names once each, in order, and nothing else", () => {
    expect(
      mentionsOf("@wilson has invoice 345 been paid? ask @ada, @wilson"),
    ).toEqual(["wilson", "ada"]);
    expect(mentionsOf("mail me at daan@example.com")).toEqual([]);
    expect(mentionsOf("")).toEqual([]);
  });
});

describe("the fold", () => {
  it("builds rooms and their messages from the store's operations", () => {
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(1, "room/close-w41", "create", "admin", {
        title: "Week 41 close",
        status: "open",
      }),
    );
    fold(
      ws,
      "rooms",
      op(2, "room/close-w41/messages/m-1", "create", "admin", {
        text: "@wilson paid?",
        mentions: ["wilson"],
      }),
    );
    fold(
      ws,
      "rooms",
      op(3, "room/close-w41/messages/m-2", "create", "wilson", {
        text: "not that I can see",
        kind: "turn",
        reply_to: "room/close-w41/messages/m-1",
      }),
    );
    fold(
      ws,
      "rooms",
      op(4, "room/other", "create", "admin", { title: "Other" }),
    );
    fold(ws, "rooms", op(5, "room/close-w41", "close", "admin", {}));
    fold(ws, "rooms", op(6, "invoice/inv-1", "create", "admin", { total: 1 })); // not a room: ignored
    const room = ws.rooms.get("room/close-w41");
    expect(room?.title).toBe("Week 41 close");
    expect(room?.status).toBe("closed");
    expect(room?.messages.map((m) => [m.author, m.kind, m.text])).toEqual([
      ["admin", "message", "@wilson paid?"],
      ["wilson", "turn", "not that I can see"],
    ]);
    expect(room?.messages[1].replyTo).toBe("room/close-w41/messages/m-1");
    expect(room?.lastAuthor).toBe("wilson");
    expect(ws.seq).toBe(6);
    // Open rooms first, then by activity.
    expect(roomOrder(ws).map((r) => r.id)).toEqual(["other", "close-w41"]);
  });
  it("names paths from subjects and ignores other stores", () => {
    expect(pathOf("rooms", "CHRON.rooms.OPS.room.r1.messages.m-1")).toBe(
      "room/r1/messages/m-1",
    );
    expect(pathOf("rooms", "CHRON.ticks.OPS.schedule.x")).toBe("");
  });
});

describe("ids and ages", () => {
  it("mints message ids and room ids", () => {
    expect(mintId(() => 0.5)).toBe("m-888888888888");
    expect(roomIdFrom("Week 41 close!")).toBe("week-41-close");
    expect(roomIdFrom("   ", () => 0.5)).toBe("r-888888888888");
  });
  it("says how long ago", () => {
    const now = Date.parse("2026-10-12T12:00:00Z");
    expect(age("2026-10-12T11:59:58Z", now)).toBe("now");
    expect(age("2026-10-12T11:58:00Z", now)).toBe("2m");
    expect(age("", now)).toBe("");
  });
});

describe("the chat's reading of the record (decision 0009)", () => {
  const seeded = () => {
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(1, `room/${GENERAL}`, "create", "admin", {
        title: "General",
        purpose: "where a workspace starts",
        status: "open",
      }),
    );
    fold(
      ws,
      "rooms",
      op(2, "room/close-w41", "create", "admin", {
        title: "Week 41 close",
        status: "open",
      }),
    );
    fold(
      ws,
      "rooms",
      op(3, "room/close-w41/messages/m-1", "create", "daan", {
        text: "@wilson paid?",
        mentions: ["wilson"],
      }),
    );
    fold(
      ws,
      "rooms",
      op(4, "room/close-w41/messages/m-2", "create", "daan", {
        text: "and 346?",
      }),
    );
    fold(
      ws,
      "rooms",
      op(5, "room/close-w41/messages/m-3", "create", "wilson", {
        text: "345 yes, 346 no",
        kind: "turn",
      }),
    );
    fold(
      ws,
      "rooms",
      op(6, "room/old", "create", "admin", { title: "Old", status: "open" }),
    );
    fold(
      ws,
      "rooms",
      op(7, "room/old/messages/m-1", "create", "daan", { text: "later" }),
    );
    fold(ws, "rooms", op(8, "room/old", "close", "admin", {}));
    return ws;
  };

  it("groups consecutive messages by one author", () => {
    const ws = seeded();
    const groups = groupMessages(ws.rooms.get("room/close-w41")!.messages);
    expect(groups.map((g) => [g[0].author, g.length])).toEqual([
      ["daan", 2],
      ["wilson", 1],
    ]);
  });

  it("opens in the open room that last had a message, else nothing — the entry, where a person starts (decision 0011)", () => {
    const ws = seeded();
    expect(lastActiveRoom(ws)?.id).toBe("close-w41"); // old had the last message but is closed
    const quiet = emptyWorkspace();
    fold(
      quiet,
      "rooms",
      op(1, `room/${GENERAL}`, "create", "admin", {
        title: "General",
        status: "open",
      }),
    );
    fold(
      quiet,
      "rooms",
      op(2, "room/other", "create", "admin", {
        title: "Other",
        status: "open",
      }),
    );
    expect(lastActiveRoom(quiet)).toBeUndefined();
    expect(lastActiveRoom(emptyWorkspace())).toBeUndefined();
  });

  it("makes General only when the record is read and holds no room", () => {
    expect(needsGeneral(emptyWorkspace(), false)).toBe(false);
    expect(needsGeneral(emptyWorkspace(), true)).toBe(true);
    expect(needsGeneral(seeded(), true)).toBe(false);
  });

  it("knows the agents the runner announced", () => {
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(1, "agent/wilson", "create", "wilson", {
        name: "wilson",
        description: "answers about invoices",
        harness: "claude-code",
        triggers: ["mention in rooms"],
        version: "0.2.0",
      }),
    );
    fold(
      ws,
      "rooms",
      op(2, "agent/wilson", "update", "wilson", {
        description: "answers about invoices and payments",
      }),
    );
    expect(ws.agents.get("wilson")).toEqual({
      name: "wilson",
      description: "answers about invoices and payments",
      harness: "claude-code",
      triggers: ["mention in rooms"],
      tools: [],
      version: "0.2.0",
    });
    expect(ws.rooms.size).toBe(0);
  });

  it("offers agents first with their line, then people, never oneself, and completes the handle", () => {
    const agents = [
      {
        name: "wilson",
        description: "answers about invoices",
        harness: "claude-code",
        triggers: [],
        tools: [],
        version: "",
      },
    ];
    const items = pickerCandidates(
      "",
      agents,
      ["wilson", "probe-sa"],
      ["daan", "ada"],
      "daan",
      (n) => n === "wilson",
    );
    expect(items.map((i) => [i.name, i.kind, i.online ?? false])).toEqual([
      ["wilson", "agent", true],
      ["probe-sa", "agent", false],
      ["ada", "person", false],
    ]);
    expect(items[0].description).toBe("answers about invoices");
    expect(
      pickerCandidates("wi", agents, [], ["daan"], "daan").map((i) => i.name),
    ).toEqual(["wilson"]);
    expect(handleBeingTyped("hello @wi")).toBe("wi");
    expect(handleBeingTyped("hello @")).toBe("");
    expect(handleBeingTyped("hello")).toBeUndefined();
    expect(handleBeingTyped("mail daan@x")).toBeUndefined();
    expect(completeHandle("hello @wi", "wilson")).toBe("hello @wilson ");
  });
});

describe("a conversation, a turn's steps and a question (decisions 0010 and 0011)", () => {
  it("names one direct conversation for a pair, whoever starts it", () => {
    expect(directId("wilson", "daan")).toBe("direct--daan--wilson");
    expect(directId("daan", "wilson")).toBe("direct--daan--wilson");
    expect(directRoom("wilson", "daan")).toEqual({
      title: "daan & wilson",
      purpose: "a direct conversation",
      status: "open",
      kind: "direct",
      participants: ["daan", "wilson"],
    });
  });

  it("folds a direct conversation, lists it for its parties only, and calls it by the other party", () => {
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(
        1,
        "room/direct--daan--wilson",
        "create",
        "daan",
        directRoom("daan", "wilson"),
      ),
    );
    fold(
      ws,
      "rooms",
      op(2, "room/direct--daan--wilson/messages/m-1", "create", "daan", {
        text: "paid?",
      }),
    );
    fold(
      ws,
      "rooms",
      op(3, "room/general", "create", "admin", {
        title: "General",
        status: "open",
      }),
    );
    fold(
      ws,
      "rooms",
      op(4, "room/general/messages/m-1", "create", "ada", { text: "later" }),
    );
    const dm = ws.rooms.get("room/direct--daan--wilson")!;
    expect(dm.kind).toBe("direct");
    expect(dm.participants).toEqual(["daan", "wilson"]);
    expect(otherParty(dm, "daan")).toBe("wilson");
    expect(titleFor(dm, "daan")).toBe("wilson");
    expect(titleFor(dm, "wilson")).toBe("daan");
    expect(conversationsOf(ws, "daan").map((r) => r.id)).toEqual([
      "direct--daan--wilson",
    ]);
    expect(conversationsOf(ws, "ada")).toEqual([]);
    expect(roomOrder(ws).map((r) => r.id)).toEqual(["general"]);
    // Where the page opens: daan's last conversation; ada's last room; for someone with nothing, the entry.
    expect(lastActiveRoom(ws, "daan")?.id).toBe("general"); // general's message is later
    fold(
      ws,
      "rooms",
      op(5, "room/direct--daan--wilson/messages/m-2", "create", "wilson", {
        text: "yes",
        kind: "turn",
      }),
    );
    expect(lastActiveRoom(ws, "daan")?.id).toBe("direct--daan--wilson");
    expect(lastActiveRoom(ws, "ada")?.id).toBe("general");
  });

  it("breaks a room out of a conversation with where it came from", () => {
    expect(
      breakoutRoom(
        "Invoice 345",
        "settle it",
        "room/direct--daan--wilson/messages/m-2",
      ),
    ).toEqual({
      title: "Invoice 345",
      purpose: "settle it",
      status: "open",
      kind: "room",
      origin: "room/direct--daan--wilson/messages/m-2",
    });
    expect(breakoutLine("Invoice 345", "room/invoice-345")).toEqual({
      text: "continued in Invoice 345 (room/invoice-345)",
      kind: "system",
    });
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(
        1,
        "room/invoice-345",
        "create",
        "daan",
        breakoutRoom(
          "Invoice 345",
          "",
          "room/direct--daan--wilson/messages/m-2",
        ),
      ),
    );
    expect(ws.rooms.get("room/invoice-345")?.origin).toBe(
      "room/direct--daan--wilson/messages/m-2",
    );
  });

  it("reads a turn's steps and usage, and says them", () => {
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(1, "room/general", "create", "admin", { title: "General" }),
    );
    fold(
      ws,
      "rooms",
      op(2, "room/general/messages/m-1", "create", "wilson", {
        text: "Two files.",
        kind: "turn",
        steps: [
          {
            tool: "Bash",
            input: "ls -la",
            output: "a.txt\nb.txt",
            ok: true,
            took_ms: 869,
          },
          { tool: "mcp__invoices__lookup", input: "345", ok: false },
        ],
        usage: {
          model: "claude-x",
          input_tokens: 34,
          output_tokens: 357,
          cost_usd: 0.19,
          turns: 2,
        },
      }),
    );
    const m = ws.rooms.get("room/general")!.messages[0];
    expect(m.steps).toEqual([
      {
        tool: "Bash",
        input: "ls -la",
        output: "a.txt\nb.txt",
        ok: true,
        tookMs: 869,
      },
      {
        tool: "mcp__invoices__lookup",
        input: "345",
        output: "",
        ok: false,
        tookMs: 0,
      },
    ]);
    expect(m.usage).toEqual({
      model: "claude-x",
      inputTokens: 34,
      outputTokens: 357,
      costUsd: 0.19,
      turns: 2,
    });
    expect(stepLine(m.steps[0])).toBe("ran Bash: ls -la");
    expect(stepLine(m.steps[1])).toBe("called invoices › lookup: 345");
    expect(costOf(m.usage)).toBe("$0.19");
    expect(
      costOf({
        model: "",
        inputTokens: 0,
        outputTokens: 0,
        costUsd: 0.001,
        turns: 1,
      }),
    ).toBe("less than a cent");
    expect(costOf(undefined)).toBe("");
  });

  it("keeps a question apart, open until a message answers it", () => {
    const ws = emptyWorkspace();
    fold(
      ws,
      "rooms",
      op(1, "room/general", "create", "admin", { title: "General" }),
    );
    fold(
      ws,
      "rooms",
      op(2, "room/general/messages/m-1", "create", "daan", { text: "run it" }),
    );
    fold(
      ws,
      "rooms",
      op(3, "room/general/messages/q-1", "create", "wilson", {
        text: "wilson wants to use Bash: date > x. Allow it?",
        kind: "question",
        choices: ["allow", "deny"],
        mentions: ["daan"],
      }),
    );
    const msgs = ws.rooms.get("room/general")!.messages;
    expect(msgs[1].kind).toBe("question");
    expect(msgs[1].choices).toEqual(["allow", "deny"]);
    expect(openQuestions(msgs).map((q) => q.path)).toEqual([
      "room/general/messages/q-1",
    ]);
    expect(groupMessages(msgs).map((g) => g.length)).toEqual([1, 1]);
    fold(
      ws,
      "rooms",
      op(4, "room/general/messages/m-2", "create", "daan", {
        text: "allow",
        reply_to: "room/general/messages/q-1",
      }),
    );
    expect(openQuestions(msgs)).toEqual([]);
    expect(answerTo(msgs, msgs[1])?.text).toBe("allow");
  });
});
