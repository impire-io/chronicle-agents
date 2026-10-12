// The composer: Enter sends, Shift+Enter breaks a line; `@` opens the
// picker — the workspace's agents with their line and online dot, then
// the account's people — and completes a handle. In a direct conversation
// the other party is addressed without an @ (decision 0011).
import { SendIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { Textarea } from "@/components/ui/textarea";
import { Refusal } from "./dialogs";
import {
  completeHandle,
  handleBeingTyped,
  pickerCandidates,
  type Agent,
  type PickerItem,
} from "./model";

export function Composer({
  placeholder,
  draft,
  onDraft,
  onSend,
  agents,
  services,
  people,
  me,
  online,
  autoFocusKey,
}: {
  placeholder: string;
  draft: string;
  onDraft: (text: string) => void;
  onSend: (text: string) => Promise<void>;
  agents: Agent[];
  services: string[];
  people: string[];
  me: string;
  online: (name: string) => boolean;
  /** Focus the composer when this changes: the room opened. */
  autoFocusKey: string;
}) {
  const [text, setTextState] = useState(draft);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [pick, setPick] = useState(0);
  const textarea = useRef<HTMLTextAreaElement>(null);

  const setText = (t: string) => {
    setTextState(t);
    onDraft(t);
  };

  useEffect(() => {
    textarea.current?.focus();
  }, [autoFocusKey]);

  const send = async () => {
    const t = text.trim();
    if (!t || busy) return;
    setBusy(true);
    setError(undefined);
    try {
      await onSend(t);
      setText("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
      textarea.current?.focus();
    }
  };

  const typed = handleBeingTyped(text);
  const candidates: PickerItem[] = useMemo(
    () =>
      typed === undefined
        ? []
        : pickerCandidates(typed, agents, services, people, me, online).slice(
            0,
            8,
          ),
    [typed, agents, services, people, me, online],
  );
  useEffect(() => setPick(0), [typed]);
  const complete = (name: string) => {
    setText(completeHandle(text, name));
    textarea.current?.focus();
  };

  return (
    <form
      className="relative grid gap-2 border-t bg-background px-4 py-3"
      onSubmit={(e) => {
        e.preventDefault();
        void send();
      }}
    >
      <Refusal>{error}</Refusal>
      {candidates.length > 0 ? (
        <ul
          className="absolute bottom-full left-4 z-10 mb-1 w-80 max-w-[calc(100%-2rem)] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
          role="listbox"
          aria-label="Who to mention"
        >
          {candidates.map((c, i) => (
            <li key={c.name}>
              <button
                type="button"
                role="option"
                aria-selected={i === pick}
                className={`flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm ${i === pick ? "bg-accent text-accent-foreground" : ""}`}
                onMouseEnter={() => setPick(i)}
                onClick={() => complete(c.name)}
              >
                {c.kind === "agent" ? (
                  <span
                    className={`size-1.5 shrink-0 rounded-full ${c.online ? "bg-primary" : "bg-border"}`}
                    aria-hidden="true"
                  />
                ) : (
                  <span className="size-1.5 shrink-0" aria-hidden="true" />
                )}
                <span className="font-medium">@{c.name}</span>
                {c.kind === "agent" ? (
                  <span className="rounded-sm bg-muted px-1 text-[10px] uppercase text-muted-foreground">
                    agent
                  </span>
                ) : null}
                {c.description ? (
                  <span className="truncate text-muted-foreground">
                    {c.description}
                  </span>
                ) : null}
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <div className="flex items-end gap-2">
        <Textarea
          ref={textarea}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (candidates.length > 0) {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setPick((p) => (p + 1) % candidates.length);
                return;
              }
              if (e.key === "ArrowUp") {
                e.preventDefault();
                setPick((p) => (p - 1 + candidates.length) % candidates.length);
                return;
              }
              if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
                e.preventDefault();
                complete(candidates[pick].name);
                return;
              }
              if (e.key === "Escape") {
                e.preventDefault();
                setText(text + " ");
                return;
              }
            }
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void send();
            }
          }}
          placeholder={placeholder}
          rows={2}
          className="min-h-10 resize-none"
          aria-label="Message"
        />
        <Button
          type="submit"
          size="icon"
          disabled={busy || !text.trim()}
          aria-label="Send"
        >
          <SendIcon />
        </Button>
      </div>
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <Kbd>Enter</Kbd> sends
        </span>
        <span className="flex items-center gap-1">
          <KbdGroup>
            <Kbd>Shift</Kbd>
            <Kbd>Enter</Kbd>
          </KbdGroup>{" "}
          breaks a line
        </span>
        <span className="flex items-center gap-1">
          <Kbd>@</Kbd> reaches an agent or a person
        </span>
      </p>
    </form>
  );
}
