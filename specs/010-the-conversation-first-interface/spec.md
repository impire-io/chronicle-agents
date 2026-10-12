# Spec 010 — the conversation-first interface: a direct conversation, a breakout, the page on the console's construction

**Work ID:** `conversation-first-interface` (chronicle-agents-hq research 002)
**Design:** `chronicle-agents-hq` @ the commit the `lands` block of
[`02-DESIGN/03-the-rooms-interface.md`](../../../chronicle-agents-hq/02-DESIGN/03-the-rooms-interface.md)
names (PR #16) —
[`03-DECISIONS/0011-a-person-starts-in-a-conversation-and-breaks-out-into-a-room.md`](../../../chronicle-agents-hq/03-DECISIONS/0011-a-person-starts-in-a-conversation-and-breaks-out-into-a-room.md),
[`02-DESIGN/01-rooms-and-messages.md`](../../../chronicle-agents-hq/02-DESIGN/01-rooms-and-messages.md)
§ the room (kind, participants, origin), § a conversation;
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md)
§ the triggers (direct), § from trigger to turn (the floor and a person);
[`02-DESIGN/03-the-rooms-interface.md`](../../../chronicle-agents-hq/02-DESIGN/03-the-rooms-interface.md)
§ the pages, § the look, § what this does not do. Built on spec 009
(`the-agents-turn`), whose signals and kinds this page renders.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **The room type gains `kind`, `participants` and `origin`** (`types/`):
  `kind` is `room` (the default) or `direct`; a direct conversation's two
  parties sorted in `participants`; a breakout's `origin`, the path of the
  message the conversation stood at. `DirectID`, `DirectPath` and
  `DirectRoom` derive the one conversation a pair has; `store init` and
  the manifest carry the revised type.
- **The `direct` trigger** (`declaration/`, `runner/`): a fifth kind — a
  message by anyone but the agent in a direct conversation the agent is a
  participant of, in the store it names; no `under`, `operation` or
  `room`. The runner folds a room's kind and participants from its
  create, rename and snapshot, matches a message in a conversation it is
  in, and composes the context as a conversation with the other party.
  `examples/wilson.yaml` declares it beside `mention`.
- **The window floor and a person** (`runner/`): a turn triggered by a
  message whose author the registry knows as a person is never refused
  by the window floor; the depth bound and the agent-to-agent rule hold.
- **The page on the console's construction** (`web/_app`): Tailwind 4
  through its Vite plugin; the shadcn/ui primitives generated for the
  `radix-nova` preset (`src/components/ui/`, `components.json` as
  chronicle-web's, lucide icons, the `cn` package); chronicle-web's
  `globals.css` verbatim as `src/index.css` with the page's additions
  (the system's colour scheme, a turn's markdown through `react-markdown`
  with GFM); `tw-animate-css`; the `@/` alias.
- **Where a person starts** (`src/entry.tsx`, `src/workspace.tsx`): the
  entry state with the agents (their line, online or offline) and the
  people, each a conversation to start, and *Open a room*; the sidebar of
  *Conversations* (the viewer's, by the other party) and *Rooms* with
  *New conversation* (a command palette, `src/dialogs.tsx`) and *New room*;
  the page opens in the viewer's last active conversation or room, else
  the entry; General is still made when the workspace has no room.
- **A conversation and a breakout**: a conversation created create-if-
  absent on first open, its head naming the other party and whether it
  is online, the composer addressing them without an `@`; *Break out into
  a room* (a dialog: title, purpose) creating the room with its origin,
  writing *continued in …* in the conversation, and opening the room;
  *Close* behind the typed confirmation.
- **The transcript** (`src/transcript.tsx`): `message-scroller`
  (follows at the bottom, stays put reading back, *Scroll to end*);
  groups as `message` with an avatar, the author, the agent badge and
  the time; `bubble` surfaces — the viewer's own in the accent, a
  person's as typed, an agent's turn as markdown; a turn's steps as a
  list that opens to each tool call and its output, the cost in the
  footer; a question as a `questionnaire` with its choices and a free
  answer, writing the answer as a message in reply, and as question and
  answer once answered; a system line as a separator `marker`; refusals
  and failures as red markers; what an agent is doing live (`src/live.ts`):
  the sentence, the steps with a spinner, the question it waits on, the
  text so far as a growing bubble.
- **The composer** (`src/composer.tsx`): Enter sends, Shift+Enter breaks
  a line, `kbd` hints; the `@` picker as before, styled as a popover.
- **Proofs**: the fold of a direct room and a breakout, the one id per
  pair, the viewer's conversations, the title from the viewer's side,
  where the page opens, steps and usage, a question and its answer, the
  cost and step lines (`src/model.test.ts`); the new signals' wire, what
  an agent is doing from picked-up through writing, steps, waiting and
  done (`src/live.test.ts`); the direct trigger and what it does not
  trigger, a person never refused by the floor, the floor on an agent's
  mention (`runner/direct_test.go`, `runner/runner_test.go`,
  `runner/live_test.go`); a direct trigger refusing `under`
  (`declaration/`). Walked in a browser against a quick start with the
  real Claude Code: the entry, a conversation started with wilson, a
  question answered from its form, the command run, the turn with its
  step and cost, a room broken out; the sidebar as a sheet at a phone's
  width.

## What this does not do

No private conversations; no receipts, edits, reactions, attachments or
search; no browser walk in the gate (the pure parts are tested; the page
was walked by hand). The console's colour scheme is chronicle-hq's.
