"use client";

import { ChevronLeft, ChevronRight, Compass, Expand, Pause, Play, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";

import { Md, StepTimeline, callOk } from "@/components/steps";
import { Badge, Button, Card, EmptyState, Kbd, PageHeader, Skeleton, cx, statusTone } from "@/components/ui";
import { SECTION_LABEL } from "@/lib/azure";
import { selfTestReport, useSelfTest } from "@/lib/hooks";

function subscribeHash(cb: () => void) {
  window.addEventListener("hashchange", cb);
  return () => window.removeEventListener("hashchange", cb);
}

/**
 * One service per slide. Steps reveal one at a time so an audience can follow
 * along; autoplay walks through everything hands-free.
 */
export function Tour() {
  const st = useSelfTest();
  const report = selfTestReport(st);
  const results = report?.results ?? [];
  const hash = useSyncExternalStore(subscribeHash, () => location.hash, () => "");
  const i = Math.max(0, Math.min(results.length - 1, (parseInt(hash.slice(1)) || 1) - 1));
  const [shown, setShown] = useState(Infinity);
  const [auto, setAuto] = useState(false);

  const r = results[i];
  const calls = r?.calls ?? [];

  const count = results.length;
  const go = useCallback(
    (n: number, reveal = false) => {
      const next = Math.max(0, Math.min(count - 1, n));
      setShown(reveal ? 0 : Infinity);
      history.replaceState(null, "", `#${next + 1}`);
      window.dispatchEvent(new HashChangeEvent("hashchange"));
      window.scrollTo({ top: 0, behavior: "smooth" });
    },
    [count],
  );

  // Autoplay: reveal steps, then move to the next service.
  useEffect(() => {
    if (!auto || !r) return;
    const t = setTimeout(
      () => {
        if (shown < calls.length) setShown((s) => (s === Infinity ? 1 : s + 1));
        else if (i < results.length - 1) go(i + 1, true);
        else setAuto(false);
      },
      shown < calls.length ? 700 : 2200,
    );
    return () => clearTimeout(t);
  }, [auto, shown, calls.length, i, results.length, go, r]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).tagName === "INPUT") return;
      if (e.key === "ArrowRight") go(i + 1);
      else if (e.key === "ArrowLeft") go(i - 1);
      else if (e.key === " ") {
        e.preventDefault();
        setAuto((a) => {
          if (!a) setShown(0);
          return !a;
        });
      } else if (e.key === "f") document.documentElement.requestFullscreen?.();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [i, go]);

  const visible = shown === Infinity ? calls : calls.slice(0, shown);
  const passedSoFar = results.slice(0, i + 1).filter((x) => x.status === "pass").length;

  return (
    <>
      <PageHeader
        icon={<Compass className="size-5" />}
        title="Guided tour"
        description="Walk through every emulated service one at a time: what is checked, each HTTP call, what was expected and what came back."
        actions={
          <>
            <Button icon={<RefreshCw className="size-4" />} loading={st.isValidating} onClick={() => st.mutate().then(() => go(0))}>
              Re-run
            </Button>
            <Button
              variant="primary"
              icon={auto ? <Pause className="size-4" /> : <Play className="size-4" />}
              disabled={!results.length}
              onClick={() => {
                if (!auto) setShown(0);
                setAuto(!auto);
              }}
            >
              {auto ? "Pause" : "Autoplay"}
            </Button>
            <Button variant="ghost" icon={<Expand className="size-4" />} onClick={() => document.documentElement.requestFullscreen?.()} aria-label="Full screen" />
          </>
        }
      />

      {!report && st.error ? (
        <Card>
          <EmptyState title="Could not run the self-test" description={st.error.message} />
        </Card>
      ) : !r ? (
        <Skeleton className="h-96 rounded-xl" />
      ) : (
        <>
          <div className="sticky top-14 z-10 -mx-4 mb-4 border-b border-border bg-bg/85 px-4 py-3 backdrop-blur sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
            <div className="flex items-center gap-3">
              <Button size="sm" variant="ghost" icon={<ChevronLeft className="size-4" />} disabled={i === 0} onClick={() => go(i - 1)}>
                Back
              </Button>
              <div className="flex flex-1 gap-1">
                {results.map((x, k) => (
                  <button
                    key={x.service}
                    title={x.service}
                    onClick={() => go(k)}
                    className={cx(
                      "h-2 flex-1 rounded-full transition-all",
                      x.status === "pass" ? "bg-ok" : "bg-bad",
                      k > i && "opacity-25",
                      k === i && "h-3 ring-2 ring-accent ring-offset-2 ring-offset-bg",
                    )}
                  />
                ))}
              </div>
              <span className="w-28 text-right text-xs tabular-nums text-muted">
                {i + 1} / {results.length} · {passedSoFar} ✓
              </span>
              <Button size="sm" variant="primary" disabled={i === results.length - 1} onClick={() => go(i + 1)}>
                Next <ChevronRight className="size-4" />
              </Button>
            </div>
          </div>

          <div key={r.service} className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_300px] animate-in">
            <Card padded={false}>
              <div className="border-b border-border p-6">
                <div className="flex flex-wrap items-center gap-2 text-[11px] font-semibold uppercase tracking-wider text-subtle">
                  <span>
                    {i + 1} of {results.length}
                  </span>
                  <span>·</span>
                  <span>{SECTION_LABEL[r.section] ?? r.section}</span>
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-3">
                  <h2 className="break-all font-mono text-2xl font-semibold tracking-tight">{r.service}</h2>
                  <Badge tone={statusTone(r.status)} dot className="px-2.5 py-1 text-xs">
                    {r.status === "pass" ? "working" : r.status}
                  </Badge>
                </div>
                {r.title && <p className="mt-2 text-base">Checked: {r.title}</p>}
                {r.notes && (
                  <p className="mt-1 text-sm text-muted">
                    Emulated: <Md text={r.notes} />
                  </p>
                )}
                {r.error && <div className="mt-3 whitespace-pre-wrap rounded-lg bg-bad-soft p-3 font-mono text-xs text-bad">{r.error}</div>}
              </div>
              <div className="p-4">
                <StepTimeline calls={visible} startOpen />
                {shown !== Infinity && shown < calls.length && (
                  <div className="py-2 pl-9 text-xs text-subtle animate-pulse-soft">
                    step {shown + 1} of {calls.length}…
                  </div>
                )}
              </div>
            </Card>

            <div className="space-y-4">
              <Card title="At a glance">
                <dl className="space-y-2.5 text-sm">
                  <Row k="HTTP calls" v={calls.length} />
                  <Row k="Expected results" v={`${calls.filter(callOk).length} / ${calls.length}`} />
                  <Row k="Extra assertions" v={calls.reduce((n, c) => n + (c.verified?.length ?? 0), 0)} />
                  <Row k="Duration" v={`${r.durationMs} ms`} />
                  <Row k="Hosts" v={[...new Set(calls.map((c) => c.url.split("/")[0].split(".")[0]))].join(", ")} />
                </dl>
              </Card>
              <Card title="Keyboard">
                <div className="space-y-2 text-xs text-muted">
                  <div className="flex justify-between">
                    previous / next <span className="flex gap-1"><Kbd>←</Kbd><Kbd>→</Kbd></span>
                  </div>
                  <div className="flex justify-between">
                    autoplay <Kbd>space</Kbd>
                  </div>
                  <div className="flex justify-between">
                    full screen <Kbd>f</Kbd>
                  </div>
                </div>
              </Card>
              <Card title="Up next">
                {results.slice(i + 1, i + 5).map((x, k) => (
                  <button key={x.service} onClick={() => go(i + 1 + k)} className="flex w-full items-center gap-2 rounded-md px-1 py-1.5 text-left text-xs hover:bg-surface-2">
                    <span className={cx("size-1.5 rounded-full", x.status === "pass" ? "bg-ok" : "bg-bad")} />
                    <span className="truncate font-mono">{x.service}</span>
                  </button>
                ))}
                {i === results.length - 1 && <div className="text-xs text-muted">That&apos;s every service. 🎉</div>}
              </Card>
            </div>
          </div>
        </>
      )}
    </>
  );
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-muted">{k}</dt>
      <dd className="font-medium tabular-nums">{v}</dd>
    </div>
  );
}
