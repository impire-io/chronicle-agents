// The transcript (design 03 § a room, decisions 0009, 0010 and 0011):
// the record's messages on a scroller that follows new ones while the
// person is at the bottom and stays put while they read back; a person's
// message and an agent's turn as messages with their author; a turn's
// steps as what the agent did; a question as a form the person answers
// in the room; and, off the record, what an agent is doing right now —
// the text as it is written, the tool it runs, the answer it waits on —
// and a refusal or a failure as a line that is plainly not a message.
import {
  BotIcon,
  CheckIcon,
  CircleAlertIcon,
  CornerDownRightIcon,
  MessageCircleQuestionIcon,
  WrenchIcon,
  XIcon,
} from "lucide-react";
import { useState } from "react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Bubble, BubbleContent } from "@/components/ui/bubble";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker";
import {
  Message,
  MessageAvatar,
  MessageContent,
  MessageFooter,
  MessageGroup,
  MessageHeader,
} from "@/components/ui/message";
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller";
import {
  Questionnaire,
  QuestionnaireActions,
  QuestionnaireChoice,
  QuestionnaireChoices,
  QuestionnaireInput,
  QuestionnaireItem,
  QuestionnaireSubmit,
  QuestionnaireTitle,
} from "@/components/ui/questionnaire";
import { Spinner } from "@/components/ui/spinner";
import {
  activitySentence,
  lineSentence,
  type Activity,
  type LiveRoom,
  type StepSignal,
} from "./live";
import { Prose } from "./markdown";
import {
  age,
  answerTo,
  costOf,
  groupMessages,
  stepLine,
  timeOf,
  type Message as Msg,
  type Room,
  type Step,
} from "./model";

export function Transcript({
  room,
  me,
  isAgent,
  live,
  now,
  hint,
  onAnswer,
}: {
  room: Room;
  me: string;
  isAgent: (name: string) => boolean;
  live: LiveRoom;
  now: number;
  /** What the empty transcript says. */
  hint: string;
  onAnswer: (question: Msg, text: string) => Promise<void>;
}) {
  const groups = groupMessages(room.messages);
  const activities = [...live.writing.entries()].sort(
    (a, b) => a[1].since - b[1].since,
  );
  return (
    <MessageScrollerProvider
      autoScroll
      defaultScrollPosition="end"
      scrollPreviousItemPeek={24}
    >
      <MessageScroller className="flex-1">
        <MessageScrollerViewport aria-label="Transcript" className="px-4 py-4">
          <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-5">
            {room.messages.length === 0 && activities.length === 0 ? (
              <Empty className="my-auto border-0">
                <EmptyHeader>
                  <EmptyTitle>Nothing said here yet</EmptyTitle>
                  <EmptyDescription>{hint}</EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : null}
            {groups.map((g) => {
              const first = g[0];
              if (first.kind === "system") {
                return (
                  <MessageScrollerItem key={first.path} messageId={first.path}>
                    <Marker variant="separator" className="text-xs">
                      <MarkerContent>{first.text}</MarkerContent>
                    </Marker>
                  </MessageScrollerItem>
                );
              }
              if (first.kind === "question") {
                return (
                  <MessageScrollerItem key={first.path} messageId={first.path}>
                    <Question
                      q={first}
                      messages={room.messages}
                      me={me}
                      now={now}
                      onAnswer={onAnswer}
                    />
                  </MessageScrollerItem>
                );
              }
              const mine = first.author === me;
              const agent = isAgent(first.author) || first.kind === "turn";
              return (
                <MessageScrollerItem
                  key={first.path}
                  messageId={first.path}
                  scrollAnchor={mine}
                >
                  <Message align={mine ? "end" : "start"}>
                    <MessageAvatar>
                      <Who name={first.author} agent={agent} />
                    </MessageAvatar>
                    <MessageContent>
                      <MessageHeader className="gap-2">
                        <span className="text-foreground">{first.author}</span>
                        {agent ? (
                          <Badge
                            variant="outline"
                            className="h-4 px-1 text-[10px] uppercase"
                          >
                            agent
                          </Badge>
                        ) : null}
                        <time
                          dateTime={first.ts}
                          title={timeOf(first.ts)}
                          className="font-normal"
                        >
                          {age(first.ts, now)}
                        </time>
                      </MessageHeader>
                      <MessageGroup>
                        {g.map((m) => (
                          <Turn key={m.path} m={m} mine={mine} agent={agent} />
                        ))}
                      </MessageGroup>
                    </MessageContent>
                  </Message>
                </MessageScrollerItem>
              );
            })}
            {activities.map(([name, a]) => (
              <MessageScrollerItem
                key={`live-${name}`}
                messageId={`live-${name}`}
              >
                <Working name={name} a={a} />
              </MessageScrollerItem>
            ))}
            {live.lines.map((l) => (
              <MessageScrollerItem key={l.id} messageId={l.id}>
                <Marker
                  variant="border"
                  className="text-destructive"
                  role="status"
                >
                  <MarkerIcon>
                    <CircleAlertIcon />
                  </MarkerIcon>
                  <MarkerContent>{lineSentence(l)}</MarkerContent>
                </Marker>
              </MessageScrollerItem>
            ))}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton direction="end" />
      </MessageScroller>
    </MessageScrollerProvider>
  );
}

function Who({ name, agent }: { name: string; agent: boolean }) {
  return (
    <Avatar className="size-8">
      <AvatarFallback className={agent ? "bg-primary/10 text-primary" : ""}>
        {agent ? (
          <BotIcon className="size-4" />
        ) : (
          name.slice(0, 1).toUpperCase() || "?"
        )}
      </AvatarFallback>
    </Avatar>
  );
}

/** One message's surface: a person's as it was typed, an agent's as markdown, with its steps when it took any. */
function Turn({ m, mine, agent }: { m: Msg; mine: boolean; agent: boolean }) {
  const variant = mine ? "default" : agent ? "muted" : "secondary";
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      {m.steps.length > 0 ? <Steps steps={m.steps} /> : null}
      <Bubble variant={variant} align={mine ? "end" : "start"}>
        <BubbleContent>
          {agent ? (
            <Prose text={m.text} />
          ) : (
            <span className="whitespace-pre-wrap">{m.text}</span>
          )}
        </BubbleContent>
      </Bubble>
      {m.replyTo || m.usage ? (
        <MessageFooter className="gap-2 font-normal">
          {m.replyTo ? (
            <span className="flex items-center gap-1">
              <CornerDownRightIcon className="size-3" />
              in reply to {m.replyTo.split("/").pop()}
            </span>
          ) : null}
          {m.usage && costOf(m.usage) ? (
            <span
              title={`${m.usage.model || "the model"}: ${String(m.usage.inputTokens)} in, ${String(m.usage.outputTokens)} out`}
            >
              {costOf(m.usage)}
              {m.usage.turns > 1 ? ` · ${String(m.usage.turns)} rounds` : ""}
            </span>
          ) : null}
        </MessageFooter>
      ) : null}
    </div>
  );
}

/** What the agent did on the way: one line per tool call, the output on request. */
function Steps({ steps }: { steps: Step[] }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="flex flex-col gap-1">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-6 w-fit px-1 text-xs text-muted-foreground"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
      >
        <WrenchIcon />
        {steps.length === 1 ? "1 step" : `${String(steps.length)} steps`}
      </Button>
      {open ? (
        <ol className="flex flex-col gap-1 border-l-2 pl-3">
          {steps.map((s, i) => (
            <li key={i} className="text-xs text-muted-foreground">
              <Marker>
                <MarkerIcon>
                  {s.ok ? (
                    <CheckIcon />
                  ) : (
                    <XIcon className="text-destructive" />
                  )}
                </MarkerIcon>
                <MarkerContent className="font-mono text-[12px] text-foreground">
                  {stepLine(s)}
                </MarkerContent>
              </Marker>
              {s.output ? (
                <pre className="mt-1 max-h-48 overflow-auto rounded-md bg-muted/50 px-2 py-1 font-mono text-[12px] whitespace-pre-wrap">
                  {s.output}
                </pre>
              ) : null}
            </li>
          ))}
        </ol>
      ) : null}
    </div>
  );
}

/** A question an agent asked: the choices as a form while nobody answered, the answer when someone did. */
function Question({
  q,
  messages,
  me,
  now,
  onAnswer,
}: {
  q: Msg;
  messages: Msg[];
  me: string;
  now: number;
  onAnswer: (question: Msg, text: string) => Promise<void>;
}) {
  const answer = answerTo(messages, q);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const asked = q.mentions.length === 0 || q.mentions.includes(me);
  return (
    <Message>
      <MessageAvatar>
        <Who name={q.author} agent />
      </MessageAvatar>
      <MessageContent>
        <MessageHeader className="gap-2">
          <span className="text-foreground">{q.author}</span>
          <Badge variant="outline" className="h-4 px-1 text-[10px] uppercase">
            asks
          </Badge>
          <time dateTime={q.ts} title={timeOf(q.ts)} className="font-normal">
            {age(q.ts, now)}
          </time>
        </MessageHeader>
        <div className="w-full max-w-xl rounded-lg border bg-card p-4">
          {answer ? (
            <div className="grid gap-2 text-sm">
              <p className="flex items-start gap-2">
                <MessageCircleQuestionIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                <span>{q.text}</span>
              </p>
              <p className="flex items-start gap-2 text-muted-foreground">
                <CheckIcon className="mt-0.5 size-4 shrink-0" />
                <span>
                  <span className="font-medium text-foreground">
                    {answer.author}
                  </span>{" "}
                  answered: {answer.text}
                </span>
              </p>
            </div>
          ) : (
            <Questionnaire
              items={[
                {
                  name: "answer",
                  choices: q.choices.map((c) => ({ value: c })),
                },
              ]}
              onSubmit={(e) => {
                e.preventDefault();
                const data = new FormData(e.currentTarget);
                const values = data
                  .getAll("answer")
                  .map(String)
                  .filter((v) => v.trim());
                const text = values[values.length - 1]?.trim() ?? "";
                if (!text) return;
                setBusy(true);
                setError(undefined);
                onAnswer(q, text)
                  .catch((err) =>
                    setError(err instanceof Error ? err.message : String(err)),
                  )
                  .finally(() => setBusy(false));
              }}
            >
              <QuestionnaireItem name="answer">
                <QuestionnaireTitle>{q.text}</QuestionnaireTitle>
                {q.choices.length > 0 ? (
                  <QuestionnaireChoices>
                    {q.choices.map((c) => (
                      <QuestionnaireChoice key={c} value={c}>
                        {c}
                      </QuestionnaireChoice>
                    ))}
                  </QuestionnaireChoices>
                ) : null}
                <QuestionnaireInput
                  placeholder={
                    q.choices.length > 0
                      ? "or answer in your own words"
                      : "Your answer"
                  }
                />
                {error ? (
                  <p className="refusal" role="alert">
                    {error}
                  </p>
                ) : null}
                <QuestionnaireActions>
                  <QuestionnaireSubmit disabled={busy}>
                    {asked ? "Answer" : "Answer for them"}
                  </QuestionnaireSubmit>
                </QuestionnaireActions>
              </QuestionnaireItem>
            </Questionnaire>
          )}
        </div>
      </MessageContent>
    </Message>
  );
}

/** What an agent is doing right now, off the record: the text so far, its steps, the answer it waits on. */
function Working({ name, a }: { name: string; a: Activity }) {
  return (
    <Message aria-live="polite">
      <MessageAvatar>
        <Who name={name} agent />
      </MessageAvatar>
      <MessageContent>
        <MessageHeader className="gap-2">
          <span className="text-foreground">{name}</span>
          <Badge variant="outline" className="h-4 px-1 text-[10px] uppercase">
            agent
          </Badge>
          <span className="flex items-center gap-1 font-normal">
            <Spinner className="size-3" />
            {activitySentence(name, a)}
          </span>
        </MessageHeader>
        {a.steps.length > 0 ? (
          <ol className="flex flex-col gap-1 border-l-2 pl-3">
            {a.steps.map((s, i) => (
              <li key={i}>
                <LiveStep s={s} />
              </li>
            ))}
          </ol>
        ) : null}
        {a.waiting ? (
          <Marker className="text-foreground">
            <MarkerIcon>
              <MessageCircleQuestionIcon />
            </MarkerIcon>
            <MarkerContent>
              waiting for an answer to: {a.waiting.text}
            </MarkerContent>
          </Marker>
        ) : null}
        {a.text ? (
          <Bubble variant="muted">
            <BubbleContent>
              <Prose text={a.text} />
            </BubbleContent>
          </Bubble>
        ) : null}
      </MessageContent>
    </Message>
  );
}

function LiveStep({ s }: { s: StepSignal }) {
  return (
    <Marker className="text-xs">
      <MarkerIcon>
        {s.phase === "started" ? (
          <Spinner className="size-3" />
        ) : s.ok ? (
          <CheckIcon />
        ) : (
          <XIcon className="text-destructive" />
        )}
      </MarkerIcon>
      <MarkerContent className="font-mono text-[12px]">
        {s.phase === "started" ? "running " : s.ok ? "ran " : "failed "}
        {s.tool}
        {s.input ? `: ${s.input}` : ""}
      </MarkerContent>
    </Marker>
  );
}
