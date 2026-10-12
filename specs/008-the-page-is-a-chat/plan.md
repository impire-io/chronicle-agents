# Plan 008 — the page is a chat

## Layout

```
web/_app/src/rooms.tsx        the chat: Rooms (sidebar, the room chosen at start, General), RoomView (transcript, live lines, composer, picker), AgentsPanel, NewRoom, Confirm
web/_app/src/live.ts          live signals, the pure parts: subjects, parsing, the room's live view (writing, lines, expiry), presence
web/_app/src/live.test.ts     vitest on them
web/_app/src/model.ts         Agent and its fold; groupMessages; GENERAL and lastActiveRoom; needsGeneral; the picker's candidates and completion; timeOf
web/_app/src/model.test.ts    vitest on the additions
web/_app/src/app.tsx          the sign-in phases only; Rooms moved out
web/_app/src/styles.css       the three-column layout, groups, the picker, the live lines, the agents panel, phone width
web/dist/                     the committed build
```

## Mechanics

1. One tail of the store feeds `fold`; a message `create` from an agent
   calls `turnLanded` on that room's live view. `listMembers` gives the
   registry's names.
2. `client.connection.subscribe` on `CHRON.LIVE.<store>.agent.*` for the
   store's life and on `CHRON.LIVE.<store>.room.<id>` for the open room's;
   a five-second tick expires stale writers and re-renders presence.
3. Once the record is read (`status` leaves *replaying*): the URL's room if
   it exists, else `needsGeneral` → create `room/general`, else
   `lastActiveRoom`.
4. The transcript's scroll is followed only while `atBottom`; switching
   rooms re-anchors and focuses the composer; drafts live in a map keyed
   by room id.

## Landing

Draft PR labelled `work/chronicle-agents-7`, after chronicle-agents-6 (the
runner's side) merges; rebased onto main, gate green, ready, merged.
Design 03 flips to `implemented` with it.
