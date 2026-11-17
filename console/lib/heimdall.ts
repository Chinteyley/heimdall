// Shapes of the report Heimdall attaches under "heimdall" (plugin/types.go).

export type Mode = "report" | "annotate" | "correct" | "check";
export type Status = "current" | "stale" | "unverifiable";

export interface Source {
  title: string;
  url: string;
  published_date?: string;
}

export interface CheckedClaim {
  id: number;
  text: string;
  quote: string;
  value: string;
  category: string;
  query: string;
  status: Status;
  freshness: number;
  current_value?: string;
  explanation: string;
  evidence_date?: string;
  sources: Source[];
}

export interface Usage {
  tavily_searches: number;
  tavily_credits: number;
  model_calls: number;
  latency_ms: number;
}

export interface Report {
  mode: Mode;
  checked_at: string;
  score: number;
  stale: number;
  claims: CheckedClaim[];
  usage: Usage;
  error?: string;
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}

export interface ChatResult {
  answer: string;
  report: Report | null;
  model: string;
  error?: string;
}
