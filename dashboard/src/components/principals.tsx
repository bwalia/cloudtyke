"use client";

import { useMemo } from "react";
import useSWR from "swr";

import { call } from "@/lib/api";
import { scopeRelation } from "@/lib/azure";

/** Resolves directory object ids to display names with one Graph getByIds call. */
export function usePrincipalNames(ids: string[]) {
  const uniq = [...new Set(ids)].sort();
  const { data } = useSWR(uniq.length ? ["getByIds", uniq.join(",")] : null, () =>
    call<{ value: { id: string; displayName?: string; "@odata.type"?: string }[] }>("graph", "/v1.0/directoryObjects/getByIds", { method: "POST", body: { ids: uniq } }),
  );
  return useMemo(
    () => Object.fromEntries((data?.value ?? []).map((o) => [o.id, { name: o.displayName ?? o.id, type: (o["@odata.type"] ?? "").replace("#microsoft.graph.", "") }])) as Record<string, { name: string; type: string }>,
    [data],
  );
}

export function ScopeCell({ assignment, viewing }: { assignment: string; viewing: string }) {
  const rel = scopeRelation(assignment, viewing);
  if (rel === "this") return <span className="text-xs font-medium">This scope</span>;
  return (
    <span className="flex flex-col text-xs">
      <span className={rel === "inherited" ? "text-muted" : "text-info"}>{rel === "inherited" ? "Inherited from" : "Child scope"}</span>
      <span className="font-mono text-[11px] text-subtle">{assignment}</span>
    </span>
  );
}

