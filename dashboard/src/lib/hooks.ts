"use client";

import { useMemo } from "react";
import useSWR from "swr";

import { API, armPath, call, useApi, useList } from "./api";
import type { RequestLogEntry, ResourceGroup, SelfTestReport, Subscription } from "./azure";

export const useSelfTest = (filter?: string) =>
  useApi<SelfTestReport>("admin", `/selftest?format=json${filter ? `&service=${encodeURIComponent(filter)}` : ""}`, {
    revalidateOnFocus: false,
    shouldRetryOnError: false,
  });

/** The self-test returns 417 when something fails; the report is still in the error body. */
export function selfTestReport(r: ReturnType<typeof useSelfTest>): SelfTestReport | undefined {
  if (r.data) return r.data;
  const body = (r.error as { body?: unknown } | undefined)?.body as SelfTestReport | undefined;
  return body?.results ? body : undefined;
}

export const useSubscriptions = () => useList<Subscription>("arm", armPath("/subscriptions"));

export function useAllResourceGroups(subs: Subscription[]) {
  const key = subs.map((s) => s.subscriptionId).join(",");
  return useSWR(key ? ["all-rgs", key] : null, async () => {
    const lists = await Promise.all(
      subs.map((s) =>
        call<{ value: ResourceGroup[] }>("arm", armPath(`/subscriptions/${s.subscriptionId}/resourcegroups`, API.resources))
          .then((r) => r.value.map((rg) => ({ ...rg, subscription: s })))
          .catch(() => []),
      ),
    );
    return lists.flat();
  });
}

export const useRequests = (limit = 200, refreshInterval = 3000) =>
  useList<RequestLogEntry>("admin", `/requests?limit=${limit}`, { refreshInterval, revalidateOnFocus: true });

interface StateDump {
  buckets: { meta?: Record<string, unknown> };
}

export interface MgNode {
  name: string;
  displayName: string;
  parent?: string;
  children: MgNode[];
  subscriptions: { subscriptionId: string; displayName: string; state: string }[];
}

/** Management-group hierarchy from the emulator's state export (ARM has no MG API in this phase). */
export function useHierarchy() {
  const r = useApi<StateDump>("admin", "/state", { revalidateOnFocus: false });
  const tree = useMemo(() => {
    const meta = r.data?.buckets?.meta ?? {};
    const mgs = new Map<string, MgNode>();
    const subs: { subscriptionId: string; displayName: string; state: string; managementGroup?: string }[] = [];
    for (const [k, v] of Object.entries(meta)) {
      if (k.startsWith("mgs/")) {
        const m = v as { name: string; displayName: string; parent?: string };
        mgs.set(m.name, { ...m, children: [], subscriptions: [] });
      } else if (k.startsWith("subscriptions/")) subs.push(v as (typeof subs)[number]);
    }
    let root: MgNode | undefined;
    for (const m of mgs.values()) {
      if (m.parent && mgs.has(m.parent)) mgs.get(m.parent)!.children.push(m);
      else root = root ?? m;
    }
    for (const s of subs) (mgs.get(s.managementGroup || "") ?? root)?.subscriptions.push(s);
    for (const m of mgs.values()) m.children.sort((a, b) => a.displayName.localeCompare(b.displayName));
    return root;
  }, [r.data]);
  return { ...r, tree };
}
