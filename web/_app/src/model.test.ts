import type { Op } from "@impire-io/chronicle";
import { describe, expect, it } from "vitest";
import { age, emptyWorkspace, fold, mentionsOf, mintId, pathOf, roomIdFrom, roomOrder } from "./model";

const op = (seq: number, path: string, type: string, author: string, payload: unknown, id = `op-${seq}`): Op => ({
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
    expect(mentionsOf("@wilson has invoice 345 been paid? ask @ada, @wilson")).toEqual(["wilson", "ada"]);
    expect(mentionsOf("mail me at daan@example.com")).toEqual([]);
    expect(mentionsOf("")).toEqual([]);
  });
});

describe("the fold", () => {
  it("builds rooms and their messages from the store's operations", () => {
    const ws = emptyWorkspace();
    fold(ws, "rooms", op(1, "room/close-w41", "create", "admin", { title: "Week 41 close", status: "open" }));
    fold(ws, "rooms", op(2, "room/close-w41/messages/m-1", "create", "admin", { text: "@wilson paid?", mentions: ["wilson"] }));
    fold(ws, "rooms", op(3, "room/close-w41/messages/m-2", "create", "wilson", { text: "not that I can see", kind: "turn", reply_to: "room/close-w41/messages/m-1" }));
    fold(ws, "rooms", op(4, "room/other", "create", "admin", { title: "Other" }));
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
    expect(pathOf("rooms", "CHRON.rooms.OPS.room.r1.messages.m-1")).toBe("room/r1/messages/m-1");
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
