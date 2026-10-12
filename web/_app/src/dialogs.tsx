// The dialogs: a new room, a breakout from a conversation, the
// irreversible close (the console's pattern: the consequence named, the
// room's name typed), and who to start a conversation with.
import { BotIcon, UserIcon } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import type { PickerItem } from "./model";

const message = (err: unknown): string =>
  err instanceof Error ? err.message : String(err);

export function Refusal({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <div className="refusal" role="alert">
      {children}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid gap-1.5 text-sm font-medium">
      {label}
      {children}
    </label>
  );
}

export function NewRoomDialog({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (title: string, purpose: string) => Promise<void>;
}) {
  const [title, setTitle] = useState("");
  const [purpose, setPurpose] = useState("");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) {
      setTitle("");
      setPurpose("");
      setError(undefined);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            setBusy(true);
            onCreate(title.trim(), purpose.trim())
              .catch((err) => setError(message(err)))
              .finally(() => setBusy(false));
          }}
        >
          <DialogHeader>
            <DialogTitle>New room</DialogTitle>
            <DialogDescription>
              A room is opened for a purpose and closed when it is served.
            </DialogDescription>
          </DialogHeader>
          <Field label="Title">
            <Input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Week 41 close"
            />
          </Field>
          <Field label="Purpose (optional)">
            <Input
              value={purpose}
              onChange={(e) => setPurpose(e.target.value)}
              placeholder="the finance week close"
            />
          </Field>
          <Refusal>{error}</Refusal>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !title.trim()}>
              Open room
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function BreakoutDialog({
  open,
  onOpenChange,
  from,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Who the conversation is with. */
  from: string;
  onCreate: (title: string, purpose: string) => Promise<void>;
}) {
  const [title, setTitle] = useState("");
  const [purpose, setPurpose] = useState("");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) {
      setTitle("");
      setPurpose("");
      setError(undefined);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            setBusy(true);
            onCreate(title.trim(), purpose.trim())
              .catch((err) => setError(message(err)))
              .finally(() => setBusy(false));
          }}
        >
          <DialogHeader>
            <DialogTitle>Break out into a room</DialogTitle>
            <DialogDescription>
              A room others can join — people and agents alike. The conversation
              with {from} stays as it is and says where it continued; the room
              says where it came from.
            </DialogDescription>
          </DialogHeader>
          <Field label="Title">
            <Input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Invoice 345"
            />
          </Field>
          <Field label="Purpose (optional)">
            <Input
              value={purpose}
              onChange={(e) => setPurpose(e.target.value)}
              placeholder="settle whether invoice 345 was paid"
            />
          </Field>
          <Refusal>{error}</Refusal>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !title.trim()}>
              Open the room
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** The irreversible act: the consequence named, the room's name typed to confirm. */
export function CloseDialog({
  open,
  onOpenChange,
  title,
  name,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  name: string;
  onConfirm: () => Promise<void>;
}) {
  const [typed, setTyped] = useState("");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) {
      setTyped("");
      setError(undefined);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (typed !== name) return;
            setBusy(true);
            onConfirm()
              .catch((err) => setError(message(err)))
              .finally(() => setBusy(false));
          }}
        >
          <DialogHeader>
            <DialogTitle>Close {title}</DialogTitle>
            <DialogDescription>
              Its messages stay, as the record they are; nobody posts in it
              again, and no agent answers in it. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <Field label={`Type ${name} to confirm`}>
            <Input
              autoFocus
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
            />
          </Field>
          <Refusal>{error}</Refusal>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={busy || typed !== name}
            >
              Close {title}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Who to start a conversation with: the agents with their line, then the people. */
export function StartConversationDialog({
  open,
  onOpenChange,
  starters,
  onStart,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  starters: PickerItem[];
  onStart: (name: string) => void;
}) {
  const agents = starters.filter((s) => s.kind === "agent");
  const people = starters.filter((s) => s.kind === "person");
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="New conversation"
      description="Who to talk to"
    >
      <Command>
        <CommandInput placeholder="Who do you want to talk to?" />
        <CommandList>
          <CommandEmpty>Nobody by that name.</CommandEmpty>
          {agents.length > 0 ? (
            <CommandGroup heading="Agents">
              {agents.map((s) => (
                <CommandItem
                  key={s.name}
                  value={`${s.name} ${s.description}`}
                  onSelect={() => onStart(s.name)}
                >
                  <BotIcon />
                  <span className="font-medium">{s.name}</span>
                  {s.description ? (
                    <span className="truncate text-muted-foreground">
                      {s.description}
                    </span>
                  ) : null}
                  <span className="ml-auto text-xs text-muted-foreground">
                    {s.online ? "online" : "offline"}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          ) : null}
          {people.length > 0 ? (
            <CommandGroup heading="People">
              {people.map((s) => (
                <CommandItem
                  key={s.name}
                  value={s.name}
                  onSelect={() => onStart(s.name)}
                >
                  <UserIcon />
                  <span className="font-medium">{s.name}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          ) : null}
        </CommandList>
      </Command>
    </CommandDialog>
  );
}
