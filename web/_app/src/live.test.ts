import { describe, expect, it } from "vitest";
import {
  activitySentence,
  agentOfSubject,
  answerLanded,
  applySignal,
  emptyLiveRoom,
  expire,
  isOnline,
  lineSentence,
  OFFLINE_AFTER_MS,
  parsePresence,
  parseRoomSignal,
  presenceSubjects,
  roomSubject,
  turnLanded,
  WRITING_TTL_MS,
  writingNames,
  writingSentence,
} from "./live";

const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));

describe("the live subjects", () => {
  it("name the store, the room and the agent under CHRON.LIVE (decision 0055)", () => {
    expect(roomSubject("rooms", "general")).toBe(
      "CHRON.LIVE.rooms.room.general",
    );
    expect(presenceSubjects("rooms")).toBe("CHRON.LIVE.rooms.agent.*");
    expect(agentOfSubject("rooms", "CHRON.LIVE.rooms.agent.wilson")).toBe(
      "wilson",
    );
    expect(agentOfSubject("rooms", "CHRON.rooms.OPS.room.general")).toBe("");
  });
});

describe("the signals' wire", () => {
  it("reads a room signal and refuses what is not one", () => {
    expect(
      parseRoomSignal(
        enc({
          agent: "wilson",
          state: "picked-up",
          trigger: "room/general/messages/m-1",
          at: "2026-10-12T15:00:00Z",
        }),
      ),
    ).toEqual({
      agent: "wilson",
      state: "picked-up",
      trigger: "room/general/messages/m-1",
      at: "2026-10-12T15:00:00Z",
      reason: "",
      text: "",
    });
    expect(
      parseRoomSignal(
        enc({
          agent: "wilson",
          state: "step",
          step: { tool: "Bash", input: "ls", phase: "started" },
        }),
      )?.step,
    ).toEqual({
      tool: "Bash",
      input: "ls",
      output: "",
      ok: false,
      phase: "started",
    });
    expect(
      parseRoomSignal(
        enc({
          agent: "wilson",
          state: "refused",
          reason: "3 turns in a minute",
        }),
      )?.reason,
    ).toBe("3 turns in a minute");
    expect(
      parseRoomSignal(enc({ agent: "wilson", state: "sleeping" })),
    ).toBeUndefined();
    expect(parseRoomSignal(enc({ state: "done" }))).toBeUndefined();
    expect(parseRoomSignal("not json")).toBeUndefined();
  });

  it("reads presence", () => {
    expect(
      parsePresence(
        enc({
          agent: "wilson",
          state: "online",
          since: "2026-10-12T15:00:00Z",
          version: "0.2.0",
        }),
      ),
    ).toEqual({
      agent: "wilson",
      state: "online",
      since: "2026-10-12T15:00:00Z",
      version: "0.2.0",
    });
    expect(parsePresence(enc({}))).toBeUndefined();
  });
});

describe("the room's live view", () => {
  const sig = (
    agent: string,
    state: "picked-up" | "thinking" | "done" | "refused" | "failed",
    reason = "",
  ) => ({
    agent,
    state,
    trigger: "room/general/messages/m-1",
    at: "",
    reason,
    text: "",
  });

  it("shows who is writing from picked-up until done, or until the turn lands", () => {
    const live = emptyLiveRoom();
    applySignal(live, sig("wilson", "picked-up"), 1000);
    applySignal(live, sig("ada", "picked-up"), 2000);
    expect(writingNames(live)).toEqual(["wilson", "ada"]);
    expect(writingSentence(writingNames(live))).toBe(
      "wilson and ada are writing…",
    );
    applySignal(live, sig("wilson", "thinking"), 3000);
    applySignal(live, sig("wilson", "done"), 4000);
    expect(writingNames(live)).toEqual(["ada"]);
    expect(writingSentence(writingNames(live))).toBe("ada is writing…");
    turnLanded(live, "ada");
    expect(writingNames(live)).toEqual([]);
    expect(writingSentence([])).toBe("");
  });

  it("turns a refusal or a failure into a line with its reason, and stops the writing", () => {
    const live = emptyLiveRoom();
    applySignal(live, sig("wilson", "picked-up"), 1000);
    applySignal(live, sig("wilson", "refused", "3 turns in a minute"), 2000);
    expect(writingNames(live)).toEqual([]);
    expect(live.lines).toHaveLength(1);
    expect(lineSentence(live.lines[0])).toBe(
      "wilson did not answer: 3 turns in a minute",
    );
    applySignal(live, sig("wilson", "failed", "the harness exited 1"), 3000);
    expect(lineSentence(live.lines[1])).toBe(
      "wilson could not answer: the harness exited 1",
    );
  });

  it("forgets a writer that never said done after the deadline", () => {
    const live = emptyLiveRoom();
    applySignal(live, sig("wilson", "picked-up"), 0);
    expire(live, WRITING_TTL_MS - 1);
    expect(writingNames(live)).toEqual(["wilson"]);
    expire(live, WRITING_TTL_MS + 1);
    expect(writingNames(live)).toEqual([]);
  });

  it("names three writers in a sentence", () => {
    expect(writingSentence(["a", "b", "c"])).toBe("a, b and c are writing…");
  });
});

describe("presence", () => {
  it("is online for thirty seconds after the last heartbeat", () => {
    expect(isOnline(undefined, 1000)).toBe(false);
    expect(isOnline(1000, 1000 + OFFLINE_AFTER_MS)).toBe(true);
    expect(isOnline(1000, 1000 + OFFLINE_AFTER_MS + 1)).toBe(false);
  });
});

describe("what an agent is doing (decision 0010)", () => {
  const at = "room/general/messages/m-1";
  it("keeps the text as it is written, each step as it starts and comes back, and the question it waits on", () => {
    const live = emptyLiveRoom();
    applySignal(
      live,
      {
        agent: "wilson",
        state: "picked-up",
        trigger: at,
        at: "",
        reason: "",
        text: "",
      },
      1000,
    );
    expect(activitySentence("wilson", live.writing.get("wilson")!)).toBe(
      "wilson is thinking…",
    );
    applySignal(
      live,
      {
        agent: "wilson",
        state: "writing",
        trigger: at,
        at: "",
        reason: "",
        text: "Let me",
      },
      1100,
    );
    applySignal(
      live,
      {
        agent: "wilson",
        state: "writing",
        trigger: at,
        at: "",
        reason: "",
        text: "Let me look.",
      },
      1200,
    );
    expect(live.writing.get("wilson")?.text).toBe("Let me look.");
    expect(activitySentence("wilson", live.writing.get("wilson")!)).toBe(
      "wilson is writing…",
    );
    applySignal(
      live,
      {
        agent: "wilson",
        state: "step",
        trigger: at,
        at: "",
        reason: "",
        text: "",
        step: {
          tool: "Bash",
          input: "ls",
          output: "",
          ok: false,
          phase: "started",
        },
      },
      1300,
    );
    expect(activitySentence("wilson", live.writing.get("wilson")!)).toBe(
      "wilson is running Bash: ls",
    );
    applySignal(
      live,
      {
        agent: "wilson",
        state: "step",
        trigger: at,
        at: "",
        reason: "",
        text: "",
        step: {
          tool: "Bash",
          input: "ls",
          output: "a.txt",
          ok: true,
          phase: "finished",
        },
      },
      1400,
    );
    expect(live.writing.get("wilson")?.steps).toEqual([
      {
        tool: "Bash",
        input: "ls",
        output: "a.txt",
        ok: true,
        phase: "finished",
      },
    ]);
    applySignal(
      live,
      {
        agent: "wilson",
        state: "waiting",
        trigger: "room/general/messages/q-1",
        at: "",
        reason: "",
        text: "Allow it?",
      },
      1500,
    );
    expect(activitySentence("wilson", live.writing.get("wilson")!)).toBe(
      "wilson is waiting for an answer",
    );
    answerLanded(live, "room/general/messages/q-1");
    expect(live.writing.get("wilson")?.waiting).toBeUndefined();
    applySignal(
      live,
      {
        agent: "wilson",
        state: "done",
        trigger: at,
        at: "",
        reason: "",
        text: "",
      },
      1600,
    );
    expect(live.writing.size).toBe(0);
  });
});
