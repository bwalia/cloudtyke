"use client";

import useSWR, { type SWRConfiguration } from "swr";

export const TENANT = "7c10d7e4-0000-4000-8000-000000000001";
export const API = {
  resources: "2024-11-01",
  auth: "2022-04-01",
  activity: "2015-04-01",
  network: "2024-05-01",
  aks: "2025-02-01",
} as const;

export type Target = "arm" | "graph" | "admin" | "login";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public body?: unknown,
  ) {
    super(message);
  }
}

/** Pulls code/message out of ARM, Graph, OAuth and admin error shapes. */
function describe(status: number, body: unknown): { code: string; message: string } {
  const b = body as Record<string, unknown> | undefined;
  const e = (b?.error ?? b) as Record<string, unknown> | string | undefined;
  if (typeof e === "string") return { code: e, message: String(b?.error_description ?? e) };
  return {
    code: String(e?.code ?? `HTTP ${status}`),
    message: String(e?.message ?? (status ? `request failed with HTTP ${status}` : "network error")),
  };
}

export interface CallInfo {
  status: number;
  durationMs: number;
  location?: string;
  body: unknown;
}

export function url(target: Target, path: string) {
  return `/api/emu/${target}${path.startsWith("/") ? path : "/" + path}`;
}

/** Calls the emulator through the dashboard's server proxy. Throws ApiError on 4xx/5xx. */
export async function call<T = unknown>(target: Target | "raw", path: string, init: { method?: string; body?: unknown } = {}): Promise<T & { __info?: CallInfo }> {
  const u = target === "raw" ? path : url(target, path);
  const res = await fetch(u, {
    method: init.method ?? "GET",
    headers: init.body !== undefined ? { "content-type": "application/json" } : undefined,
    body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
    cache: "no-store",
  });
  const text = await res.text();
  let body: unknown = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (!res.ok) {
    const d = describe(res.status, body);
    throw new ApiError(res.status, d.code, d.message, body);
  }
  const info: CallInfo = {
    status: res.status,
    durationMs: Number(res.headers.get("x-ct-duration-ms") || 0),
    location: res.headers.get("x-ct-azure-asyncoperation") || res.headers.get("x-ct-location") || undefined,
    body,
  };
  if (body && typeof body === "object") Object.defineProperty(body, "__info", { value: info, enumerable: false });
  return (body ?? { __info: info }) as T & { __info?: CallInfo };
}

/** Waits for an ARM async operation (202 + Location / Azure-AsyncOperation) to finish. */
export async function waitAsync(info: CallInfo | undefined, timeoutMs = 60_000) {
  if (!info || info.status !== 202 || !info.location) return;
  const until = Date.now() + timeoutMs;
  while (Date.now() < until) {
    await new Promise((r) => setTimeout(r, 400));
    const res = await fetch(info.location, { cache: "no-store" });
    if (res.status === 202) continue;
    const body = await res.json().catch(() => null);
    const st = body?.status;
    if (st === "InProgress" || st === "Running" || st === "Accepted") continue;
    if (!res.ok || st === "Failed" || st === "Canceled") {
      const d = describe(res.status, body);
      throw new ApiError(res.status, d.code, d.message, body);
    }
    return;
  }
  throw new ApiError(0, "Timeout", "async operation did not finish in time");
}

export type ListResponse<T> = { value: T[] } & Record<string, unknown>;

/** SWR hook for a GET through the proxy. key=null disables the request. */
export function useApi<T = unknown>(target: Target, path: string | null, cfg?: SWRConfiguration) {
  return useSWR<T, ApiError>(path === null ? null : [target, path], ([t, p]: [Target, string]) => call<T>(t, p), {
    revalidateOnFocus: true,
    keepPreviousData: true,
    ...cfg,
  });
}

export function useList<T>(target: Target, path: string | null, cfg?: SWRConfiguration) {
  const r = useApi<ListResponse<T>>(target, path, cfg);
  return { ...r, items: r.data?.value ?? [] };
}

export function armPath(p: string, apiVersion: string = API.resources) {
  return `${p}${p.includes("?") ? "&" : "?"}api-version=${apiVersion}`;
}
