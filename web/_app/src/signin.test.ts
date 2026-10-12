import type { IdentityMembership } from "@impire-io/chronicle";
import { describe, expect, it } from "vitest";
import { chooseAccount, readReturn, siteOf } from "./signin";

const pending = {
  state: "s1",
  verifier: "v",
  redirectUri: "https://agents.example/",
};

describe("GitHub's return", () => {
  it("is nothing when the page was not sent back", () => {
    expect(readReturn("", pending)).toEqual({ kind: "none" });
    expect(readReturn("?room=x", undefined)).toEqual({ kind: "none" });
  });
  it("is the code when the state is this tab's", () => {
    expect(readReturn("?code=abc&state=s1", pending)).toEqual({
      kind: "code",
      code: "abc",
      pending,
    });
  });
  it("is refused when the state is not this tab's, or there was no sign-in", () => {
    expect(readReturn("?code=abc&state=other", pending).kind).toBe("refused");
    expect(readReturn("?code=abc&state=s1", undefined).kind).toBe("refused");
  });
  it("carries GitHub's own refusal", () => {
    expect(
      readReturn(
        "?error=access_denied&error_description=The+user+said+no",
        pending,
      ),
    ).toEqual({
      kind: "refused",
      why: "GitHub said: The user said no",
    });
  });
});

const m = (account: string, role = "writer"): IdentityMembership => ({
  account,
  principal: "daan",
  role,
});

describe("the account choice", () => {
  it("enters the one account without asking", () => {
    expect(chooseAccount([m("daan")], undefined, undefined)).toEqual({
      kind: "enter",
      account: "daan",
    });
  });
  it("offers several, unless one was remembered", () => {
    expect(chooseAccount([m("daan"), m("acme")], undefined, undefined)).toEqual(
      { kind: "choose" },
    );
    expect(chooseAccount([m("daan"), m("acme")], undefined, "acme")).toEqual({
      kind: "enter",
      account: "acme",
    });
    expect(chooseAccount([m("daan"), m("acme")], undefined, "gone")).toEqual({
      kind: "choose",
    });
  });
  it("says when there is no account", () => {
    expect(chooseAccount([], undefined, undefined)).toEqual({ kind: "none" });
  });
  it("enters a pinned account the identity holds, and refuses one it does not", () => {
    expect(chooseAccount([m("daan"), m("acme")], "acme", "daan")).toEqual({
      kind: "enter",
      account: "acme",
    });
    expect(chooseAccount([m("daan")], "acme", undefined).kind).toBe("refused");
  });
});

describe("the site", () => {
  it("is the profile's origin", () => {
    expect(
      siteOf("https://chronicle.impire.dev/.well-known/chronicle/profile.json"),
    ).toBe("https://chronicle.impire.dev");
    expect(siteOf("not a url")).toBeUndefined();
  });
});
