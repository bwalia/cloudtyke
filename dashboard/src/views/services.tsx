"use client";

import { Boxes, Compass, Play, RefreshCw, Search, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { Md, StepTimeline, callOk } from "@/components/steps";
import { Badge, Button, Card, EmptyState, PageHeader, Skeleton, StatusDot, cx, statusTone } from "@/components/ui";
import { useList } from "@/lib/api";
import { SECTION_LABEL, ago, type CoverageRow, type SelfTestResult } from "@/lib/azure";
import { selfTestReport, useSelfTest } from "@/lib/hooks";

export function Services() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const st = useSelfTest();
  const report = selfTestReport(st);
  const coverage = useList<CoverageRow>("admin", "/coverage", { revalidateOnFocus: false });
  const [q, setQ] = useState("");
  const section = params.get("section");
  const selected = params.get("id");

  useEffect(() => {
    if (params.get("run") === "1") {
      st.mutate();
      router.replace(pathname);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const results = useMemo(() => report?.results ?? [], [report]);
  const filtered = results.filter(
    (r) => (!section || r.section === section) && (!q || `${r.service} ${r.title} ${r.notes}`.toLowerCase().includes(q.toLowerCase())),
  );
  const current = results.find((r) => r.service === selected) ?? filtered[0];
  const apiVersions = coverage.items.find((c) => c.type === current?.service)?.apiVersions;

  const select = (id: string) => {
    const p = new URLSearchParams(params);
    p.set("id", id);
    router.replace(`${pathname}?${p}`, { scroll: false });
  };
  const setSection = (s: string | null) => {
    const p = new URLSearchParams(params);
    if (s) p.set("section", s);
    else p.delete("section");
    p.delete("id");
    router.replace(`${pathname}?${p}`, { scroll: false });
  };

  const sections = [...new Set(results.map((r) => r.section))];

  return (
    <>
      <PageHeader
        icon={<Boxes className="size-5" />}
        title="Services"
        description="Every row of the emulator's coverage matrix with a live end-to-end check: happy path, the error codes Terraform and the SDKs rely on, and RBAC / Graph permission enforcement."
        actions={
          <>
            <Link href="/tour">
              <Button icon={<Compass className="size-4" />}>Guided tour</Button>
            </Link>
            <Button variant="primary" icon={<Play className="size-4" />} loading={st.isValidating} onClick={() => st.mutate()}>
              Run all checks
            </Button>
          </>
        }
      />

      {report && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <FilterChip active={!section} onClick={() => setSection(null)}>
            All <span className="tabular-nums opacity-70">{results.length}</span>
          </FilterChip>
          {sections.map((s) => {
            const rs = results.filter((r) => r.section === s);
            const bad = rs.some((r) => r.status !== "pass");
            return (
              <FilterChip key={s} active={section === s} onClick={() => setSection(s)}>
                <StatusDot tone={bad ? "bad" : "ok"} />
                {SECTION_LABEL[s] ?? s} <span className="tabular-nums opacity-70">{rs.length}</span>
              </FilterChip>
            );
          })}
          <span className="ml-auto text-xs text-subtle">
            {report.passed}/{report.passed + report.failed} passing · {report.durationMs} ms · {ago(report.started)}
          </span>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-[360px_1fr]">
        <div className="overflow-hidden rounded-xl border border-border bg-surface shadow-card lg:sticky lg:top-20 lg:max-h-[calc(100vh-6rem)] lg:overflow-y-auto">
          <div className="sticky top-0 z-10 border-b border-border bg-surface p-2">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-subtle" />
              <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter services…" className="h-8 w-full rounded-lg bg-surface-2/60 pl-8 pr-3 text-sm placeholder:text-subtle focus:outline-none" />
            </div>
          </div>
          {!report ? (
            <div className="space-y-2 p-3">
              {Array.from({ length: 10 }).map((_, i) => (
                <Skeleton key={i} className="h-10" />
              ))}
            </div>
          ) : (
            (section ? [section] : sections).map((s) => {
              const rs = filtered.filter((r) => r.section === s);
              if (!rs.length) return null;
              return (
                <div key={s}>
                  <div className="bg-surface-2/50 px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-subtle">{SECTION_LABEL[s] ?? s}</div>
                  {rs.map((r) => (
                    <button
                      key={r.service}
                      onClick={() => select(r.service)}
                      className={cx(
                        "flex w-full items-center gap-2.5 border-l-2 px-3 py-2 text-left transition-colors",
                        current?.service === r.service ? "border-accent bg-accent-soft/50" : "border-transparent hover:bg-surface-2/60",
                      )}
                    >
                      <StatusDot tone={statusTone(r.status)} />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-mono text-[12.5px] font-medium">{r.service}</span>
                        <span className="block truncate text-[11px] text-subtle">{r.title}</span>
                      </span>
                      <span className="text-[10px] tabular-nums text-subtle">{r.calls?.length ?? 0}</span>
                    </button>
                  ))}
                </div>
              );
            })
          )}
        </div>

        <div className="min-w-0">
          {!report && st.error ? (
            <Card>
              <EmptyState icon={<TriangleAlert className="size-5" />} title="Could not run the self-test" description={st.error.message} />
            </Card>
          ) : !current ? (
            <Card>
              {report ? <EmptyState title="No services match" /> : <Skeleton className="h-64" />}
            </Card>
          ) : (
            <ServiceDetail r={current} apiVersions={apiVersions} rerun={() => st.mutate()} running={st.isValidating} />
          )}
        </div>
      </div>
    </>
  );
}

function FilterChip({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={cx(
        "inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors",
        active ? "border-accent bg-accent-soft text-accent" : "border-border bg-surface text-muted hover:text-fg",
      )}
    >
      {children}
    </button>
  );
}

function ServiceDetail({ r, apiVersions, rerun, running }: { r: SelfTestResult; apiVersions?: string[]; rerun: () => void; running: boolean }) {
  const calls = r.calls ?? [];
  const verified = calls.reduce((n, c) => n + (c.verified?.length ?? 0), 0);
  return (
    <div className="space-y-4">
      <section className="rounded-xl border border-border bg-surface p-5 shadow-card">
        <div className="flex flex-wrap items-start gap-3">
          <div className="min-w-0 flex-1">
            <div className="text-[11px] font-semibold uppercase tracking-wider text-subtle">{SECTION_LABEL[r.section] ?? r.section}</div>
            <h2 className="mt-1 break-all font-mono text-xl font-semibold">{r.service}</h2>
            {r.title && <p className="mt-1 text-sm text-fg">Checks: {r.title}</p>}
            {r.notes && (
              <p className="mt-1 text-sm text-muted">
                Emulated: <Md text={r.notes} />
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Badge tone={statusTone(r.status)} dot className="px-2.5 py-1 text-xs">
              {r.status === "pass" ? "working" : r.status}
            </Badge>
            <Button size="sm" icon={<RefreshCw className="size-3.5" />} loading={running} onClick={rerun}>
              Re-run
            </Button>
          </div>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Mini label="HTTP calls" value={calls.length} />
          <Mini label="Assertions" value={calls.length + verified} />
          <Mini label="Duration" value={`${r.durationMs} ms`} />
          <Mini label="Unexpected results" value={calls.filter((c) => !callOk(c)).length} />
        </div>
        {apiVersions && apiVersions.length > 0 && (
          <div className="mt-4 flex flex-wrap items-center gap-1.5">
            <span className="mr-1 text-xs text-muted">api-versions</span>
            {apiVersions.map((v) => (
              <Badge key={v} tone="accent">
                {v}
              </Badge>
            ))}
          </div>
        )}
        {r.error && <div className="mt-4 whitespace-pre-wrap rounded-lg bg-bad-soft p-3 font-mono text-xs text-bad">{r.error}</div>}
      </section>
      <Card title="What was tested" actions={<span className="text-xs text-subtle">click a step for its request and response</span>}>
        {calls.length ? <StepTimeline calls={calls} startOpen /> : <EmptyState title="No calls recorded" />}
      </Card>
    </div>
  );
}

function Mini({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-lg bg-surface-2/60 px-3 py-2">
      <div className="text-[11px] text-muted">{label}</div>
      <div className="text-base font-semibold tabular-nums">{value}</div>
    </div>
  );
}
