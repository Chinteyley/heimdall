import type { CheckedClaim, Report } from "@/lib/heimdall";

const statusLabel = { current: "Current", stale: "Out of date", unverifiable: "Unverified" } as const;
const statusTone = {
  current: "text-emerald-300 border-emerald-400/40 bg-emerald-400/10",
  stale: "text-rose-300 border-rose-400/40 bg-rose-500/10",
  unverifiable: "text-amber-200 border-amber-300/40 bg-amber-300/10",
} as const;

function scoreTone(score: number) {
  if (score >= 60) return "#34d399";
  if (score >= 30) return "#fcd34d";
  return "#fb7185";
}

// ScoreDial draws the report's freshness (its lowest claim score) as a ring.
export function ScoreDial({ score, size = 96 }: { score: number; size?: number }) {
  const r = size / 2 - 6;
  const c = 2 * Math.PI * r;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-label={`Freshness ${score} of 100`}>
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="#27272a" strokeWidth={6} />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        stroke={scoreTone(score)}
        strokeWidth={6}
        strokeLinecap="round"
        strokeDasharray={`${(c * score) / 100} ${c}`}
        transform={`rotate(-90 ${size / 2} ${size / 2})`}
        style={{ transition: "stroke-dasharray 600ms cubic-bezier(0.2, 0.8, 0.2, 1)" }}
      />
      <text x="50%" y="46%" dy="0.35em" textAnchor="middle" className="fill-zinc-50 font-mono" fontSize={size / 3.4}>
        {score}
      </text>
      <text x="50%" y="70%" textAnchor="middle" className="fill-zinc-400 font-mono" fontSize={size / 9}>
        / 100
      </text>
    </svg>
  );
}

export function ReportPanel({
  report,
  selected,
  onSelect,
}: {
  report: Report;
  selected: number | null;
  onSelect: (id: number) => void;
}) {
  if (report.error) {
    return <p className="rounded-lg border border-rose-400/40 bg-rose-500/10 p-4 text-sm text-rose-200">Check failed: {report.error}</p>;
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-4">
        <ScoreDial score={report.score} />
        <div className="text-sm leading-6 text-zinc-300">
          <p className="text-base font-medium text-zinc-50">
            {report.claims.length === 0
              ? "Nothing time-sensitive"
              : `${report.stale} of ${report.claims.length} claim${report.claims.length === 1 ? "" : "s"} out of date`}
          </p>
          <p className="font-mono text-xs text-zinc-400">
            {report.usage.tavily_searches} Tavily searches · {report.usage.model_calls} model calls ·{" "}
            {(report.usage.latency_ms / 1000).toFixed(1)} s
          </p>
        </div>
      </div>
      <ul className="flex flex-col gap-2">
        {report.claims.map((c) => (
          <ClaimCard key={c.id} claim={c} active={selected === c.id} onSelect={() => onSelect(c.id)} />
        ))}
      </ul>
    </div>
  );
}

function ClaimCard({ claim, active, onSelect }: { claim: CheckedClaim; active: boolean; onSelect: () => void }) {
  return (
    <li>
      <button
        onClick={onSelect}
        className={`w-full rounded-xl border p-3 text-left transition-colors ${active ? "border-zinc-300/60 bg-zinc-800/80" : "border-zinc-800 bg-zinc-900/60 hover:border-zinc-700"}`}
      >
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <span className={`rounded-full border px-2 py-0.5 text-xs font-medium ${statusTone[claim.status]}`}>
            {statusLabel[claim.status]}
          </span>
          <span className="font-mono text-xs text-zinc-400">
            {claim.category} · <span className="text-zinc-100">{claim.freshness}</span>
          </span>
        </div>
        <p className="text-sm leading-6 text-zinc-100">{claim.text}</p>
        {claim.status === "stale" && claim.current_value && (
          <p className="mt-1 text-sm leading-6">
            <span className="text-rose-300 line-through">{claim.value || "then"}</span>{" "}
            <span className="font-medium text-emerald-300">→ {claim.current_value}</span>
          </p>
        )}
        {active && (
          <div className="mt-2 flex flex-col gap-1.5 border-t border-zinc-800 pt-2">
            <p className="text-sm leading-6 text-zinc-300">{claim.explanation}</p>
            {claim.sources.map((s) => (
              <a
                key={s.url}
                href={s.url}
                target="_blank"
                rel="noopener noreferrer"
                className="truncate text-xs text-sky-300 underline underline-offset-2"
              >
                {s.title || s.url}
                {s.published_date ? ` · ${s.published_date}` : ""}
              </a>
            ))}
          </div>
        )}
      </button>
    </li>
  );
}
