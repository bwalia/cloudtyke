"use client";

import { ArrowRight, Ban, CheckCircle2, Globe, Route as RouteIcon, ShieldAlert, ShieldCheck, Waypoints, XCircle } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { call } from "@/lib/api";
import { Badge, Button, Card, Field, Input, Select, cx } from "./ui";

export interface Host {
  label: string;
  ip: string;
  subnetId: string;
}

interface Hop {
  step: number;
  where: string;
  action: string;
  detail: string;
  verdict: "ok" | "blocked" | "dropped" | "egress";
  route?: { addressPrefix: string; nextHopType: string; nextHopIpAddress?: string; source: string; name?: string };
  rule?: { nsg: string; rule: string; priority: number; access: string };
}

interface TraceResult {
  result: "Delivered" | "Blocked" | "Dropped" | "Internet" | "Unknown";
  summary: string;
  hops: Hop[];
}

const resultTone = { Delivered: "ok", Internet: "info", Blocked: "bad", Dropped: "bad", Unknown: "warn" } as const;

function hopIcon(h: Hop) {
  if (h.verdict === "blocked") return <ShieldAlert className="size-4" />;
  if (h.verdict === "dropped") return <Ban className="size-4" />;
  if (h.verdict === "egress") return <Globe className="size-4" />;
  if (h.action.startsWith("NSG") || h.action.startsWith("Security admin")) return <ShieldCheck className="size-4" />;
  if (h.action === "Route") return <RouteIcon className="size-4" />;
  if (h.action === "Peering" || h.action === "Appliance") return <Waypoints className="size-4" />;
  if (h.action === "Delivered") return <CheckCircle2 className="size-4" />;
  return <ArrowRight className="size-4" />;
}

/**
 * Pick a source and destination, see the packet's path: which route matched,
 * which peering or appliance it crossed, which NSG / admin rule decided.
 */
export function TracePanel({ hosts }: { hosts: Host[] }) {
  // deep link: /network?from=10.1.1.4&to=10.2.1.4&port=22 prefills and runs the trace
  const params = useSearchParams();
  const byIP = (ip: string | null) => hosts.find((h) => h.ip === ip)?.subnetId ?? "";
  const [from, setFrom] = useState(byIP(params.get("from")));
  const [to, setTo] = useState(params.get("to") && !byIP(params.get("to")) ? "custom" : byIP(params.get("to")));
  const [custom, setCustom] = useState(params.get("to") ?? "");
  const [port, setPort] = useState(params.get("port") ?? "443");
  const [proto, setProto] = useState("Tcp");
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<TraceResult | null>(null);
  const src = hosts.find((h) => h.subnetId === from) ?? hosts[0];
  const dstIP = to === "custom" ? custom : (hosts.find((h) => h.subnetId === to) ?? hosts[1])?.ip;
  const ready = useMemo(() => !!src && !!dstIP, [src, dstIP]);

  const run = async () => {
    if (!src || !dstIP) return;
    setBusy(true);
    try {
      const q = new URLSearchParams({ from: src.ip, to: dstIP, port, protocol: proto, fromSubnet: src.subnetId });
      setRes(await call<TraceResult>("admin", `/network/trace?${q}`));
    } catch (e) {
      setRes({ result: "Unknown", summary: e instanceof Error ? e.message : String(e), hops: [] });
    } finally {
      setBusy(false);
    }
  };

  const auto = params.get("from") && params.get("to");
  useEffect(() => {
    if (!auto || !hosts.length) return;
    const t = setTimeout(() => void run(), 0);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [auto, hosts.length]);

  return (
    <Card title="Trace a packet" actions={<span className="text-xs text-subtle">effective routes, peerings, appliances, admin rules and NSGs, like Network Watcher</span>}>
      <div className="grid gap-3 md:grid-cols-[1fr_1fr_110px_110px_auto] md:items-end">
        <Field label="From (a VM in)">
          <Select value={src?.subnetId ?? ""} onChange={(e) => setFrom(e.target.value)}>
            {hosts.map((h) => (
              <option key={h.subnetId} value={h.subnetId}>
                {h.label} · {h.ip}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="To">
          <div className="flex gap-2">
            <Select value={to || hosts[1]?.subnetId || ""} onChange={(e) => setTo(e.target.value)}>
              {hosts.map((h) => (
                <option key={h.subnetId} value={h.subnetId}>
                  {h.label} · {h.ip}
                </option>
              ))}
              <option value="custom">Other IP…</option>
            </Select>
            {to === "custom" && <Input value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="8.8.8.8" className="w-36" />}
          </div>
        </Field>
        <Field label="Port">
          <Input value={port} onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))} />
        </Field>
        <Field label="Protocol">
          <Select value={proto} onChange={(e) => setProto(e.target.value)}>
            <option>Tcp</option>
            <option>Udp</option>
            <option>Icmp</option>
          </Select>
        </Field>
        <Button variant="primary" loading={busy} disabled={!ready} onClick={run} icon={<Waypoints className="size-4" />}>
          Trace
        </Button>
      </div>

      {res && (
        <div className="mt-5 animate-in">
          <div className={cx("flex items-start gap-3 rounded-lg p-3", res.result === "Delivered" || res.result === "Internet" ? "bg-ok-soft" : res.result === "Unknown" ? "bg-warn-soft" : "bg-bad-soft")}>
            {res.result === "Delivered" || res.result === "Internet" ? <CheckCircle2 className="mt-0.5 size-5 text-ok" /> : <XCircle className="mt-0.5 size-5 text-bad" />}
            <div>
              <Badge tone={resultTone[res.result]}>{res.result}</Badge>
              <p className="mt-1 text-sm">{res.summary}</p>
            </div>
          </div>
          <ol className="mt-4 space-y-0">
            {res.hops.map((h, i) => (
              <li key={h.step} className="relative flex gap-3 pb-4">
                {i < res.hops.length - 1 && <span className="absolute left-[15px] top-8 h-[calc(100%-2rem)] w-px bg-border" />}
                <span
                  className={cx(
                    "z-10 grid size-8 shrink-0 place-items-center rounded-full border-2 bg-surface",
                    h.verdict === "ok" ? "border-ok text-ok" : h.verdict === "egress" ? "border-info text-info" : "border-bad text-bad",
                  )}
                >
                  {hopIcon(h)}
                </span>
                <div className="min-w-0 flex-1 pt-1">
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-semibold">{h.action}</span>
                    <span className="font-mono text-[11px] text-subtle">{h.where}</span>
                  </div>
                  <p className="mt-0.5 text-sm text-muted">{h.detail}</p>
                  <div className="mt-1 flex flex-wrap gap-1.5">
                    {h.route && (
                      <Badge tone={h.route.source === "User" ? "warn" : "neutral"}>
                        {h.route.source === "User" ? "UDR" : "system"} {h.route.addressPrefix} → {h.route.nextHopType}
                        {h.route.nextHopIpAddress ? ` ${h.route.nextHopIpAddress}` : ""}
                      </Badge>
                    )}
                    {h.rule && (
                      <Badge tone={h.rule.access === "Deny" ? "bad" : "ok"}>
                        {h.rule.nsg} · {h.rule.rule} · {h.rule.priority} · {h.rule.access}
                      </Badge>
                    )}
                  </div>
                </div>
              </li>
            ))}
          </ol>
        </div>
      )}
    </Card>
  );
}

/** Test hosts: the .4 address of every subnet, like the first VM Azure would place there. */
export function hostsFrom(vnets: { name: string; properties: { subnets?: { id: string; name: string; properties: { addressPrefix?: string; addressPrefixes?: string[] } }[] } }[]): Host[] {
  const out: Host[] = [];
  for (const v of vnets)
    for (const s of v.properties.subnets ?? []) {
      const cidr = s.properties.addressPrefix ?? s.properties.addressPrefixes?.[0];
      if (!cidr) continue;
      const [ip] = cidr.split("/");
      const parts = ip.split(".").map(Number);
      parts[3] += 4;
      out.push({ label: `${v.name}/${s.name}`, ip: parts.join("."), subnetId: s.id });
    }
  return out;
}
