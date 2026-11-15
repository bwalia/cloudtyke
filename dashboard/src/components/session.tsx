"use client";

import useSWR, { useSWRConfig } from "swr";

export interface Session {
  persona: "owner" | "reader";
  domain: string;
  tenantId: string;
  personas: { id: "owner" | "reader"; name: string; role: string; clientId: string }[];
  current: { id: string; name: string; role: string; clientId: string };
  claims: Record<string, unknown> | null;
  error: string | null;
}

export function useSession() {
  return useSWR<Session>("session", () => fetch("/api/session", { cache: "no-store" }).then((r) => r.json()), { revalidateOnFocus: false });
}

/** Switches identity and refetches everything, so every view reflects the new RBAC. */
export function useSwitchPersona() {
  const { mutate } = useSWRConfig();
  return async (persona: "owner" | "reader") => {
    await fetch("/api/session", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ persona }) });
    await mutate(() => true, undefined, { revalidate: true });
  };
}
