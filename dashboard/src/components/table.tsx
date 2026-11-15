"use client";

import { ArrowDown, ArrowUp, Search } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import type { ApiError } from "@/lib/api";
import { EmptyState, ErrorState, SkeletonRows, cx } from "./ui";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  sort?: (row: T) => string | number;
  className?: string;
}

/** Searchable, sortable table with loading / error / empty states built in. */
export function DataTable<T>({
  rows,
  columns,
  rowKey,
  onRowClick,
  loading,
  error,
  retry,
  searchText,
  searchPlaceholder = "Filter…",
  empty,
  toolbar,
  selectedKey,
}: {
  rows: T[];
  columns: Column<T>[];
  rowKey: (r: T) => string;
  onRowClick?: (r: T) => void;
  loading?: boolean;
  error?: ApiError | Error;
  retry?: () => void;
  searchText?: (r: T) => string;
  searchPlaceholder?: string;
  empty?: ReactNode;
  toolbar?: ReactNode;
  selectedKey?: string | null;
}) {
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<{ key: string; dir: 1 | -1 } | null>(null);

  const shown = useMemo(() => {
    let r = rows;
    if (q && searchText) {
      const needle = q.toLowerCase();
      r = r.filter((x) => searchText(x).toLowerCase().includes(needle));
    }
    const col = sort && columns.find((c) => c.key === sort.key);
    if (col?.sort) {
      const f = col.sort;
      r = [...r].sort((a, b) => {
        const x = f(a);
        const y = f(b);
        return (x < y ? -1 : x > y ? 1 : 0) * sort!.dir;
      });
    }
    return r;
  }, [rows, q, sort, columns, searchText]);

  return (
    <div className="overflow-hidden rounded-xl border border-border bg-surface shadow-card">
      {(searchText || toolbar) && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
          {searchText && (
            <div className="relative min-w-[220px] flex-1">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-subtle" />
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder={searchPlaceholder}
                className="h-8 w-full rounded-lg bg-transparent pl-8 pr-3 text-sm placeholder:text-subtle focus:bg-surface-2 focus:outline-none"
              />
            </div>
          )}
          <div className="flex items-center gap-2 text-xs text-subtle">
            {!loading && (
              <span className="tabular-nums">
                {shown.length}
                {shown.length !== rows.length ? ` of ${rows.length}` : ""} item{rows.length === 1 ? "" : "s"}
              </span>
            )}
            {toolbar}
          </div>
        </div>
      )}
      {error ? (
        <ErrorState error={error} retry={retry} />
      ) : loading && !rows.length ? (
        <SkeletonRows />
      ) : !shown.length ? (
        empty ?? <EmptyState title={q ? "No matches" : "Nothing here yet"} description={q ? `Nothing matches “${q}”.` : undefined} />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs font-medium text-muted">
                {columns.map((c) => (
                  <th key={c.key} className={cx("px-4 py-2.5 font-medium whitespace-nowrap", c.className)}>
                    {c.sort ? (
                      <button
                        className="inline-flex items-center gap-1 hover:text-fg"
                        onClick={() => setSort((s) => (s?.key === c.key ? (s.dir === 1 ? { key: c.key, dir: -1 } : null) : { key: c.key, dir: 1 }))}
                      >
                        {c.header}
                        {sort?.key === c.key && (sort.dir === 1 ? <ArrowUp className="size-3" /> : <ArrowDown className="size-3" />)}
                      </button>
                    ) : (
                      c.header
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => {
                const k = rowKey(r);
                return (
                  <tr
                    key={k}
                    onClick={onRowClick ? () => onRowClick(r) : undefined}
                    className={cx(
                      "border-b border-border last:border-0 transition-colors",
                      onRowClick && "cursor-pointer hover:bg-surface-2/70",
                      selectedKey === k && "bg-accent-soft/60",
                    )}
                  >
                    {columns.map((c) => (
                      <td key={c.key} className={cx("px-4 py-2.5 align-middle", c.className)}>
                        {c.render(r)}
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
