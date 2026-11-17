import type { ChatMessage, ChatResult, Mode } from "@/lib/heimdall";

const BIFROST_URL = process.env.BIFROST_URL ?? "http://localhost:8080";
const BIFROST_VK = process.env.BIFROST_VK ?? "";
const MODES: Mode[] = ["report", "annotate", "correct", "check"];

// Forwards one chat turn to Bifrost with the chosen x-heimdall mode, and
// splits the reply into the answer text and Heimdall's report.
export async function POST(request: Request) {
  const { messages, model, mode } = (await request.json()) as {
    messages: ChatMessage[];
    model: string;
    mode: Mode;
  };
  if (!Array.isArray(messages) || !MODES.includes(mode) || typeof model !== "string") {
    return Response.json({ error: "bad request" }, { status: 400 });
  }

  const res = await fetch(`${BIFROST_URL}/v1/chat/completions`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "x-heimdall": mode, "x-bf-vk": BIFROST_VK },
    body: JSON.stringify({ model, messages }),
  });
  const body = await res.json().catch(() => null);
  if (!res.ok || !body) {
    const error = body?.error?.message ?? `Bifrost returned HTTP ${res.status}`;
    return Response.json({ error } satisfies Partial<ChatResult>, { status: 502 });
  }
  const result: ChatResult = {
    answer: mode === "check" ? "" : (body.choices?.[0]?.message?.content ?? ""),
    report: body.heimdall ?? null,
    model,
  };
  return Response.json(result);
}
