# Plan 010 — the conversation-first interface

## Layout

```
types/types.go                       RoomKindRoom, RoomKindDirect; RoomData.Kind, Participants, Origin; DirectID, DirectPath, DirectRoom; the revised room schema
declaration/declaration.go           KindDirect and its validation; Describe
runner/runner.go                     the room's kind and parties folded; the direct match; the window floor and a person; the conversation's context
runner/direct_test.go                the direct trigger, and what it does not trigger
examples/wilson.yaml                 the direct trigger declared
web/_app/package.json                tailwindcss, @tailwindcss/vite, @shadcn/react, cn, cmdk, radix-ui, class-variance-authority, lucide-react, react-markdown, remark-gfm, tw-animate-css; shadcn and @types/node as dev
web/_app/components.json             chronicle-web's (radix-nova, neutral, lucide, the @/ aliases)
web/_app/vite.config.ts              the Tailwind plugin, the @/ alias
web/_app/tsconfig.json               the @/ paths
web/_app/src/index.css               chronicle-web's globals.css, plus the system scheme and the markdown classes
web/_app/src/components/ui/*.tsx     the generated shadcn/ui primitives (never edited by hand; regenerated with the CLI)
web/_app/src/hooks/use-mobile.ts     chronicle-web's
web/_app/src/model.ts                Room.kind/participants/origin; Message.choices/steps/usage; direct ids and rooms; conversationsOf, roomOrder, titleFor, otherParty; breakoutRoom, breakoutLine; openQuestions, answerTo; costOf, stepLine
web/_app/src/live.ts                 writing, step, waiting; Activity; activitySentence; answerLanded
web/_app/src/app.tsx                 the sign-in phases on the primitives; Workspace
web/_app/src/workspace.tsx           the connected page: the tail, presence, the room's live subject, where it opens, the sidebar, the head, the dialogs, send and answer
web/_app/src/entry.tsx               where a person starts
web/_app/src/transcript.tsx          the transcript on message-scroller; Turn, Steps, Question, Working
web/_app/src/composer.tsx            the composer and the @ picker
web/_app/src/dialogs.tsx             NewRoomDialog, BreakoutDialog, CloseDialog, StartConversationDialog, Refusal
web/_app/src/markdown.tsx            Prose
web/dist/                            the committed build
```

## Mechanics

1. The page keeps one tail of the store; a message create that is a turn
   ends the agent's live activity, one that answers a question ends the
   waiting. The open room's live subject is subscribed while it is open.
2. Starting a conversation creates `room/direct--<a>--<b>` create-if-absent
   and opens it; a breakout creates the room with `origin`, writes the
   system line, and opens it.
3. The runner's `direct` match reads the room's kind and parties from its
   projection (`foldRoomOp`), so a conversation created before the runner
   started is known from the history it replays.

## Landing

Draft PR labelled `work/conversation-first-interface`, after
chronicle-agents-hq PR #16 and chronicle-agents PR #9 (spec 009) merge;
rebased onto main, gate green, ready, merged. Design 03 flips to
`implemented` with it.
