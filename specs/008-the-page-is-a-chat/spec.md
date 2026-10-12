# Spec 008 — the page is a chat

**Work ID:** `chronicle-agents-7` (tracker chronicle-agents-7)
**Design:** `chronicle-agents-hq` @ `af1bb2b` (PR #14) —
[`03-DECISIONS/0009-the-rooms-page-is-a-chat.md`](../../../chronicle-agents-hq/03-DECISIONS/0009-the-rooms-page-is-a-chat.md),
[`02-DESIGN/03-the-rooms-interface.md`](../../../chronicle-agents-hq/02-DESIGN/03-the-rooms-interface.md)
§ the pages (one page a chat, rooms, a room, live lines, writing, agents),
§ the look, § what this does not do;
[`02-DESIGN/01-rooms-and-messages.md`](../../../chronicle-agents-hq/02-DESIGN/01-rooms-and-messages.md)
§ the room (*General*), § the workspace (the `agent` type);
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md)
§ live signals. The runner's side — the `agent` type, the announcement, the
signals, *General* at `store init` — is spec 007 (chronicle-agents-6), not
this spec's.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **One page, a chat** (`web/_app/src/rooms.tsx`): the room list as a
  sidebar with *New room* at its head, the active room's transcript in
  the middle, the agents panel beside it (a toggle on narrow screens), the
  composer at the bottom. The page opens in the room that last had a
  message, else *General*; a workspace with no room at all gets *General*
  from the page itself, create-if-absent (an exists refusal is fine).
- **The transcript, as a chat reads it**: anchored to the bottom — it
  follows new messages while the person is at the bottom and stays put
  while they read back — consecutive messages by one author grouped under
  one name, the time on hover, the sequence and the rest in a small menu
  per message; an agent's turn visibly an agent's (the author is a service
  account in the registry, or the runner announced it).
- **Live lines**, off the record: the page subscribes to the room's live
  subject (`CHRON.LIVE.<store>.room.<id>`) and every agent's presence
  (`CHRON.LIVE.<store>.agent.*`) over the SDK's connection. *wilson is
  writing…* sits beneath the transcript from *picked-up* until *done* or a
  turn from wilson lands, and is forgotten after five minutes without; a
  *refused* or a *failed* shows as a muted line in the transcript with the
  reason in the person's words, gone when the room is next opened; a
  heartbeat within thirty seconds is *online*.
- **The composer**: Enter sends, Shift+Enter breaks a line, the draft is
  kept per room while the person switches. Typing `@` opens the picker:
  the workspace's agents first with their description and online dot
  (announced ones, then service accounts the registry knows), then the
  account's people; arrow keys move, Tab and Enter complete the handle,
  Escape dismisses. `mentions` is filled from the `@name`s in the text.
- **The agents panel**: the workspace's agents from the store's `agent`
  instances — name, description, what it reacts to on hover, *online* or
  *offline* from presence — service accounts no runner announced below
  them as *not announced*, and the account's people under *People*.
- **Proofs** (`web/_app/src/model.test.ts`, `live.test.ts`): the fold of
  `agent/<name>`, grouping, the room the page opens in, when *General* is
  made, the picker's ordering and completion, the live subjects and the
  signals' wire, the writing state from picked-up to done or a landed
  turn, refusals and failures as lines, the deadline, presence.

## What this does not do

No read receipts, no unread counts, no edits, no deletions, no reactions,
no attachments, no search within a room (design 03 § what this does not
do). No browser walk in the gate; the pure parts are tested, the page is
exercised by hand against the quick start.
