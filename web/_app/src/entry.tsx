// Where a person starts (decision 0011): nothing chosen yet, so the page
// offers who to talk to — the workspace's agents with their line and
// whether they are online, then the account's people — and the rooms as
// the other way in.
import { BotIcon, HashIcon, MessagesSquareIcon, UserIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import type { PickerItem } from "./model";

export function Entry({
  starters,
  reading,
  onStart,
  onNewRoom,
}: {
  starters: PickerItem[];
  reading: boolean;
  onStart: (name: string) => void;
  onNewRoom: () => void;
}) {
  const agents = starters.filter((s) => s.kind === "agent");
  const people = starters.filter((s) => s.kind === "person");
  return (
    <div className="flex flex-1 items-center justify-center overflow-y-auto p-6">
      <Empty className="w-full max-w-2xl">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <MessagesSquareIcon />
          </EmptyMedia>
          <EmptyTitle>
            {reading ? "Reading the record…" : "Start a conversation"}
          </EmptyTitle>
          <EmptyDescription>
            {reading
              ? "The workspace's conversations and rooms are being read."
              : "Talk to an agent or a person directly. When more people or agents are needed, break the conversation out into a room."}
          </EmptyDescription>
        </EmptyHeader>
        {!reading ? (
          <EmptyContent className="w-full max-w-none gap-6">
            {agents.length > 0 ? (
              <Starters title="Agents" items={agents} onStart={onStart} />
            ) : (
              <p className="text-sm text-muted-foreground">
                No agent is in this workspace yet. One appears when it runs:{" "}
                <code className="font-mono text-[13px]">
                  chronicle-agents run wilson.yaml
                </code>
              </p>
            )}
            {people.length > 0 ? (
              <Starters title="People" items={people} onStart={onStart} />
            ) : null}
            <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
              <span>or</span>
              <Button variant="outline" size="sm" onClick={onNewRoom}>
                <HashIcon />
                Open a room
              </Button>
            </div>
          </EmptyContent>
        ) : null}
      </Empty>
    </div>
  );
}

function Starters({
  title,
  items,
  onStart,
}: {
  title: string;
  items: PickerItem[];
  onStart: (name: string) => void;
}) {
  return (
    <section className="w-full text-left" aria-label={title}>
      <h2 className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title}
      </h2>
      <ul className="grid gap-2 sm:grid-cols-2">
        {items.map((s) => (
          <li key={s.name}>
            <button
              type="button"
              onClick={() => onStart(s.name)}
              className="flex w-full items-start gap-3 rounded-md border bg-card px-3 py-2.5 text-left transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
              <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                {s.kind === "agent" ? (
                  <BotIcon className="size-4" />
                ) : (
                  <UserIcon className="size-4" />
                )}
              </span>
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2">
                  <span className="truncate font-medium">{s.name}</span>
                  {s.kind === "agent" ? (
                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                      <span
                        className={`size-1.5 rounded-full ${s.online ? "bg-primary" : "bg-border"}`}
                        aria-hidden="true"
                      />
                      {s.online ? "online" : "offline"}
                    </span>
                  ) : null}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {s.description ||
                    (s.kind === "agent"
                      ? "an agent"
                      : "a person in this account")}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
