"use client";

import { ChevronRight } from "lucide-react";
import { useState, type ReactNode } from "react";

import { CopyButton, cx } from "./ui";

/** Collapsible, syntax-coloured JSON. Strings that hold JSON (self-test bodies) are parsed first. */
export function JsonView({ value, className, collapsedDepth = 3, maxHeight = "28rem" }: { value: unknown; className?: string; collapsedDepth?: number; maxHeight?: string }) {
  let v = value;
  if (typeof v === "string") {
    try {
      v = JSON.parse(v);
    } catch {
      /* plain text */
    }
  }
  const text = typeof v === "string" ? v : JSON.stringify(v, null, 2);
  return (
    <div className={cx("group relative rounded-lg border border-border bg-surface-2/60", className)}>
      <CopyButton value={text ?? ""} className="absolute right-1.5 top-1.5 opacity-0 transition-opacity group-hover:opacity-100" />
      <pre className="overflow-auto p-3 font-mono text-[12px] leading-[1.55]" style={{ maxHeight }}>
        {typeof v === "string" ? <span className="whitespace-pre-wrap break-all">{v}</span> : <Node v={v} depth={0} collapsedDepth={collapsedDepth} />}
      </pre>
    </div>
  );
}

function Node({ v, depth, collapsedDepth, k, last = true }: { v: unknown; depth: number; collapsedDepth: number; k?: string; last?: boolean }) {
  const [open, setOpen] = useState(depth < collapsedDepth);
  const comma = last ? "" : ",";
  const key = k !== undefined ? (
    <>
      <span className="json-key">&quot;{k}&quot;</span>:{" "}
    </>
  ) : null;

  if (v === null || typeof v !== "object") {
    let el: ReactNode;
    if (typeof v === "string") el = <span className="json-str break-all">&quot;{v}&quot;</span>;
    else if (typeof v === "number") el = <span className="json-num">{v}</span>;
    else el = <span className="json-lit">{String(v)}</span>;
    return (
      <div style={{ paddingLeft: depth ? 14 : 0 }}>
        {key}
        {el}
        {comma}
      </div>
    );
  }
  const isArr = Array.isArray(v);
  const entries = isArr ? (v as unknown[]).map((x, i) => [String(i), x] as const) : Object.entries(v as Record<string, unknown>);
  const [o, c] = isArr ? ["[", "]"] : ["{", "}"];
  if (!entries.length)
    return (
      <div style={{ paddingLeft: depth ? 14 : 0 }}>
        {key}
        {o}
        {c}
        {comma}
      </div>
    );
  return (
    <div style={{ paddingLeft: depth ? 14 : 0 }}>
      <button type="button" onClick={() => setOpen(!open)} className="-ml-3.5 inline-flex items-center text-subtle hover:text-fg">
        <ChevronRight className={cx("size-3 transition-transform", open && "rotate-90")} />
      </button>
      {key}
      {o}
      {!open && (
        <button type="button" onClick={() => setOpen(true)} className="mx-1 rounded bg-border px-1 text-[10px] text-muted">
          {entries.length} {isArr ? "items" : "keys"}
        </button>
      )}
      {open && entries.map(([ek, ev], i) => <Node key={ek} k={isArr ? undefined : ek} v={ev} depth={depth + 1} collapsedDepth={collapsedDepth} last={i === entries.length - 1} />)}
      {c}
      {comma}
    </div>
  );
}
