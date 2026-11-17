import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { visit, SKIP } from "unist-util-visit";
import type { Element, ElementContent, Root, Text } from "hast";
import type { CheckedClaim } from "@/lib/heimdall";

// rehypeMarkClaims wraps each claim's verbatim quote in a <mark>, so the answer
// shows which sentences Heimdall checked and what it found.
function rehypeMarkClaims(claims: CheckedClaim[]) {
  return () => (tree: Root) => {
    const pending = claims.filter((c) => c.quote);
    visit(tree, "text", (node: Text, index, parent) => {
      if (!parent || index === undefined || (parent as Element).tagName === "mark") return;
      const claim = pending.find((c) => node.value.includes(c.quote));
      if (!claim) return;
      pending.splice(pending.indexOf(claim), 1);
      const at = node.value.indexOf(claim.quote);
      const parts: ElementContent[] = [];
      if (at > 0) parts.push({ type: "text", value: node.value.slice(0, at) });
      parts.push({
        type: "element",
        tagName: "mark",
        properties: { dataClaim: String(claim.id), dataStatus: claim.status },
        children: [{ type: "text", value: claim.quote }],
      });
      const rest = node.value.slice(at + claim.quote.length);
      if (rest) parts.push({ type: "text", value: rest });
      parent.children.splice(index, 1, ...parts);
      return [SKIP, index + parts.length];
    });
  };
}

// rehypeTagCorrections tags a <strong> that directly follows a <del> (Heimdall's
// "~~old~~ **new**"), so only corrections are coloured, not the model's own bold text.
function rehypeTagCorrections() {
  return (tree: Root) => {
    visit(tree, "element", (node: Element) => {
      let prev: ElementContent | undefined;
      for (const child of node.children) {
        if (child.type === "text" && !child.value.trim()) continue;
        if (child.type === "element" && child.tagName === "strong" && prev?.type === "element" && prev.tagName === "del") {
          child.properties = { ...child.properties, dataCorrection: "true" };
        }
        prev = child;
      }
    });
  };
}

const markTone: Record<string, string> = {
  current: "bg-emerald-400/15 decoration-emerald-400",
  stale: "bg-rose-500/20 decoration-rose-400",
  unverifiable: "bg-amber-300/15 decoration-amber-300",
};

export function Answer({
  text,
  claims,
  selected,
  onSelect,
}: {
  text: string;
  claims: CheckedClaim[];
  selected: number | null;
  onSelect: (id: number) => void;
}) {
  return (
    <div className="answer text-[15px] leading-7 text-zinc-100">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeMarkClaims(claims), rehypeTagCorrections]}
        components={{
          mark: ({ node, children }) => {
            const id = Number(node?.properties?.dataClaim);
            const status = String(node?.properties?.dataStatus);
            return (
              <mark
                onClick={() => onSelect(id)}
                className={`cursor-pointer rounded-sm px-0.5 text-zinc-50 underline decoration-2 underline-offset-4 transition-colors ${markTone[status]} ${selected === id ? "ring-1 ring-zinc-100/60" : ""}`}
              >
                {children}
              </mark>
            );
          },
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer" className="text-sky-300 underline underline-offset-2">
              {children}
            </a>
          ),
          del: ({ children }) => <del className="text-rose-300/80 decoration-rose-400">{children}</del>,
          strong: ({ node, children }) => (
            <strong className={node?.properties?.dataCorrection ? "font-semibold text-emerald-300" : "font-semibold text-zinc-50"}>
              {children}
            </strong>
          ),
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}
