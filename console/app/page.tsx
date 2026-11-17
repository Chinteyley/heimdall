"use client";

import { useState } from "react";
import { Answer } from "@/components/answer";
import { ReportPanel } from "@/components/claims";
import type { ChatMessage, ChatResult, Mode, Report } from "@/lib/heimdall";

const MODES = [
  { id: "report", label: "Report", hint: "Answer untouched, report attached" },
  { id: "annotate", label: "Annotate", hint: "Stale values struck through inline" },
  { id: "correct", label: "Correct", hint: "Answer rewritten from the evidence" },
] as const satisfies { id: Mode; label: string; hint: string }[];

// An older model answers from older training data, which is the point of the demo.
const MODELS = ["openai/gpt-4.1-mini", "openai/gpt-4.1", "openai/gpt-5.4-mini"] as const;

const PROMPTS = [
  "What does the OpenAI API charge for GPT-4o, per million tokens?",
  "Who is the CEO of Intel, and what's the latest Next.js version?",
  "When do the EU AI Act's rules for general-purpose AI models apply?",
] as const;

interface Turn {
  question: string;
  result: ChatResult | null;
}

type Tab = "chat" | "check";

export default function Home() {
  const [tab, setTab] = useState<Tab>("chat");
  const [mode, setMode] = useState<Mode>("annotate");
  const [model, setModel] = useState<string>(MODELS[0]);
  const [input, setInput] = useState("");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [checkText, setCheckText] = useState("");
  const [checkReport, setCheckReport] = useState<Report | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<number | null>(null);

  const latest = turns.at(-1)?.result?.report ?? null;
  const panelReport = tab === "chat" ? latest : checkReport;

  async function call(messages: ChatMessage[], callMode: Mode): Promise<ChatResult | null> {
    setBusy(true);
    setError(null);
    setSelected(null);
    try {
      const res = await fetch("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ messages, model, mode: callMode }),
      });
      const data = (await res.json()) as ChatResult;
      if (!res.ok) {
        setError(data.error ?? `HTTP ${res.status}`);
        return null;
      }
      return data;
    } catch (e) {
      setError(e instanceof Error ? e.message : "request failed");
      return null;
    } finally {
      setBusy(false);
    }
  }

  async function ask(question: string) {
    const q = question.trim();
    if (!q || busy) return;
    setInput("");
    const history: ChatMessage[] = turns.flatMap((t) =>
      t.result ? [{ role: "user" as const, content: t.question }, { role: "assistant" as const, content: t.result.answer }] : [],
    );
    setTurns((prev) => [...prev, { question: q, result: null }]);
    const result = await call([...history, { role: "user", content: q }], mode);
    setTurns((prev) => {
      if (!result) return prev.slice(0, -1);
      const next = [...prev];
      next[next.length - 1] = { question: q, result };
      return next;
    });
  }

  async function check() {
    if (!checkText.trim() || busy) return;
    const result = await call([{ role: "user", content: checkText }], "check");
    setCheckReport(result?.report ?? null);
  }

  return (
    <div className="flex min-h-screen flex-col bg-zinc-950 text-zinc-50">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 px-4 py-3 sm:px-6">
        <div className="flex items-baseline gap-3">
          <h1 className="text-lg font-semibold tracking-tight">Heimdall</h1>
          <p className="text-sm text-zinc-400">a freshness gate for Bifrost</p>
        </div>
        <nav className="flex rounded-lg border border-zinc-800 p-0.5 text-sm">
          {(["chat", "check"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTab(t)}
              className={`rounded-md px-3 py-1 ${tab === t ? "bg-zinc-100 text-zinc-950" : "text-zinc-300 hover:text-zinc-50"}`}
            >
              {t === "chat" ? "Chat through Bifrost" : "Check an answer"}
            </button>
          ))}
        </nav>
      </header>

      <main className="grid flex-1 grid-cols-1 lg:grid-cols-[minmax(0,1fr)_400px]">
        <section className="flex min-h-0 flex-col border-zinc-800 lg:border-r">
          {tab === "chat" ? (
            <>
              <div className="flex flex-wrap items-center gap-2 border-b border-zinc-800 px-4 py-2.5 sm:px-6">
                <div className="flex rounded-lg border border-zinc-800 p-0.5 text-sm" role="radiogroup" aria-label="Heimdall mode">
                  {MODES.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      role="radio"
                      aria-checked={mode === m.id}
                      title={m.hint}
                      onClick={() => setMode(m.id)}
                      className={`rounded-md px-3 py-1 ${mode === m.id ? "bg-zinc-800 text-zinc-50" : "text-zinc-400 hover:text-zinc-100"}`}
                    >
                      {m.label}
                    </button>
                  ))}
                </div>
                <label className="flex items-center gap-2 text-sm text-zinc-400">
                  Model
                  <select
                    value={model}
                    onChange={(e) => setModel(e.target.value)}
                    className="rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1 font-mono text-xs text-zinc-100"
                  >
                    {MODELS.map((m) => (
                      <option key={m}>{m}</option>
                    ))}
                  </select>
                </label>
              </div>

              <div className="flex-1 space-y-8 overflow-y-auto px-4 py-6 sm:px-6">
                {turns.length === 0 && (
                  <div className="mx-auto max-w-xl space-y-4 pt-10 text-center">
                    <p className="text-2xl font-semibold tracking-tight">Ask something that changes.</p>
                    <p className="text-sm leading-6 text-zinc-400">
                      Every answer crosses Bifrost, and Heimdall checks its prices, versions, names and laws against
                      today&apos;s web with Tavily.
                    </p>
                    <div className="flex flex-col gap-2 pt-2">
                      {PROMPTS.map((p) => (
                        <button
                          key={p}
                          type="button"
                          onClick={() => ask(p)}
                          className="rounded-xl border border-zinc-800 bg-zinc-900/60 px-4 py-3 text-left text-sm text-zinc-200 hover:border-zinc-700"
                        >
                          {p}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
                {turns.map((t, i) => (
                  <article key={`${i}-${t.question}`} className="space-y-3">
                    <p className="ml-auto w-fit max-w-[85%] rounded-2xl bg-zinc-800 px-4 py-2.5 text-[15px] leading-6">
                      {t.question}
                    </p>
                    {t.result ? (
                      <Answer
                        text={t.result.answer}
                        claims={t.result.report?.mode === "report" ? t.result.report.claims : []}
                        selected={i === turns.length - 1 ? selected : null}
                        onSelect={setSelected}
                      />
                    ) : (
                      <p className="animate-pulse text-sm text-zinc-400">Answering, then checking against today…</p>
                    )}
                  </article>
                ))}
              </div>

              <form
                className="flex gap-2 border-t border-zinc-800 p-3 sm:px-6"
                onSubmit={(e) => {
                  e.preventDefault();
                  ask(input);
                }}
              >
                <input
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  placeholder="Ask about a price, a version, who runs what…"
                  aria-label="Message"
                  className="flex-1 rounded-xl border border-zinc-800 bg-zinc-900 px-4 py-2.5 text-[15px] text-zinc-50 placeholder:text-zinc-500 focus:border-zinc-600 focus:outline-none"
                />
                <button
                  type="submit"
                  disabled={busy || !input.trim()}
                  className="rounded-xl bg-zinc-100 px-4 text-sm font-medium text-zinc-950 disabled:opacity-40"
                >
                  Send
                </button>
              </form>
            </>
          ) : (
            <div className="flex flex-1 flex-col gap-3 p-4 sm:p-6">
              <label htmlFor="check-text" className="text-sm text-zinc-300">
                Paste any AI-generated answer. Heimdall finds its time-sensitive claims and checks each one live.
              </label>
              <textarea
                id="check-text"
                value={checkText}
                onChange={(e) => setCheckText(e.target.value)}
                rows={14}
                className="flex-1 resize-none rounded-xl border border-zinc-800 bg-zinc-900 p-4 font-mono text-sm leading-6 text-zinc-100 focus:border-zinc-600 focus:outline-none"
              />
              <button
                type="button"
                onClick={check}
                disabled={busy || !checkText.trim()}
                className="self-start rounded-xl bg-zinc-100 px-4 py-2 text-sm font-medium text-zinc-950 disabled:opacity-40"
              >
                {busy ? "Checking…" : "Check freshness"}
              </button>
            </div>
          )}
        </section>

        <aside className="border-t border-zinc-800 p-4 sm:p-6 lg:border-t-0">
          <h2 className="mb-4 text-xs font-medium uppercase tracking-[0.14em] text-zinc-400">Freshness</h2>
          {error && <p className="mb-4 rounded-lg border border-rose-400/40 bg-rose-500/10 p-3 text-sm text-rose-200">{error}</p>}
          {busy && !panelReport && <p className="animate-pulse text-sm text-zinc-400">Searching with Tavily…</p>}
          {panelReport ? (
            <ReportPanel report={panelReport} selected={selected} onSelect={setSelected} />
          ) : (
            !busy && <p className="text-sm leading-6 text-zinc-400">Claims and their sources show up here.</p>
          )}
        </aside>
      </main>
    </div>
  );
}
