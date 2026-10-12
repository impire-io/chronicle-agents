// A turn is written in markdown, as a coding agent writes: headings,
// lists, code. Rendered without raw HTML (react-markdown's default), so a
// message is text, never markup.
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function Prose({ text }: { text: string }) {
  return (
    <div className="prose-chat">
      <Markdown remarkPlugins={[remarkGfm]}>{text}</Markdown>
    </div>
  );
}
