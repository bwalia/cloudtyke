"use client";

import { Check, ChevronRight, X } from "lucide-react";
import { useState } from "react";

import type { SelfTestCall } from "@/lib/azure";
import { JsonView } from "./json";
import { Badge, cx, statusTone } from "./ui";

export function callOk(c: SelfTestCall) {
  if (!c.expect) return c.status < 400;
  return c.expect.split(" or ").some((e) => e.startsWith(String(c.status)));
}

const methodTone: Record<string, string> = {
  GET: "text-info",
  POST: "text-ok",
  PUT: "text-warn",
  PATCH: "text-warn",
  DELETE: "text-bad",
  HEAD: "text-muted",
};

/** Turns `code` spans from coverage notes into <code>. */
export function Md({ text }: { text: string }) {
  return (
    <>
      {text.split("`").map((p, i) =>
        i % 2 ? (
          <code key={i} className="rounded bg-surface-2 px-1 py-px font-mono text-[0.9em]">
            {p}
          </code>
        ) : (
          <span key={i}>{p}</span>
        ),
      )}
    </>
  );
}

/** Every HTTP call a self-test check made, as an expandable timeline. */
export function StepTimeline({ calls, startOpen = false }: { calls: SelfTestCall[]; startOpen?: boolean }) {
  return (
    <ol className="relative">
      {calls.map((c, i) => (
        <Step key={i} c={c} n={i + 1} last={i === calls.length - 1} startOpen={startOpen && !callOk(c)} />
      ))}
    </ol>
  );
}

function Step({ c, n, last, startOpen }: { c: SelfTestCall; n: number; last: boolean; startOpen: boolean }) {
  const [open, setOpen] = useState(startOpen);
  const ok = callOk(c);
  const [host, ...rest] = c.url.split("/");
  return (
    <li className="relative min-w-0 pl-9">
      {!last && <span className="absolute bottom-0 left-[13px] top-7 w-px bg-border" />}
      <span className={cx("absolute left-0 top-2 grid size-[27px] place-items-center rounded-full border-2 bg-surface", ok ? "border-ok text-ok" : "border-bad text-bad")}>
        {ok ? <Check className="size-3.5" strokeWidth={3} /> : <X className="size-3.5" strokeWidth={3} />}
      </span>
      <button onClick={() => setOpen(!open)} className="group w-full rounded-lg px-2 py-2 text-left transition-colors hover:bg-surface-2/70">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 text-sm font-medium">
              <span className="text-[11px] tabular-nums text-subtle">{n}</span>
              <span className="first-letter:uppercase">{c.step || `${c.method} request`}</span>
            </div>
            <div className="mt-0.5 flex min-w-0 items-center gap-1.5 font-mono text-[11.5px]">
              <span className={cx("font-bold", methodTone[c.method] ?? "text-muted")}>{c.method}</span>
              <span className="text-subtle">{host}</span>
              <span className="truncate text-muted">/{rest.join("/")}</span>
            </div>
            {c.verified && c.verified.length > 0 && (
              <div className="mt-1.5 flex flex-wrap gap-1">
                {c.verified.map((v, i) => (
                  <span key={i} className="inline-flex items-center gap-1 rounded-md bg-ok-soft px-1.5 py-0.5 text-[11px] text-ok">
                    <Check className="size-3" strokeWidth={3} />
                    {v}
                  </span>
                ))}
              </div>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-2 pt-0.5">
            {c.expect && <span className="hidden text-[11px] text-subtle sm:inline">expect {c.expect}</span>}
            <Badge tone={ok ? statusTone(c.status) : "bad"}>
              {c.status}
              {c.error ? ` ${c.error}` : ""}
            </Badge>
            <ChevronRight className={cx("size-4 text-subtle transition-transform", open && "rotate-90")} />
          </div>
        </div>
      </button>
      {open && (
        <div className="grid gap-3 px-2 pb-3 lg:grid-cols-2">
          <div>
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-subtle">Request</div>
            <JsonView value={c.request || "(no body)"} maxHeight="20rem" />
          </div>
          <div>
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-subtle">Response · {c.status}</div>
            <JsonView value={c.response || "(no body)"} maxHeight="20rem" />
          </div>
        </div>
      )}
    </li>
  );
}
