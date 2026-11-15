"use client";

import { Check, Copy, Inbox, Loader2, RefreshCw, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";

import type { ApiError } from "@/lib/api";

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(" ");
}

type Variant = "primary" | "secondary" | "ghost" | "danger";
const variants: Record<Variant, string> = {
  primary: "bg-accent text-accent-fg hover:brightness-110 shadow-card",
  secondary: "bg-surface text-fg border border-border hover:border-border-strong hover:bg-surface-2 shadow-card",
  ghost: "text-muted hover:text-fg hover:bg-surface-2",
  danger: "bg-bad text-white hover:brightness-110 shadow-card",
};

export function Button({
  variant = "secondary",
  size = "md",
  loading,
  icon,
  className,
  children,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: "sm" | "md"; loading?: boolean; icon?: ReactNode }) {
  return (
    <button
      {...rest}
      disabled={rest.disabled || loading}
      className={cx(
        "inline-flex items-center justify-center gap-1.5 rounded-lg font-medium transition-all disabled:opacity-50 disabled:pointer-events-none whitespace-nowrap",
        size === "sm" ? "h-7 px-2.5 text-xs" : "h-9 px-3.5 text-sm",
        variants[variant],
        className,
      )}
    >
      {loading ? <Loader2 className="size-4 animate-spin" /> : icon}
      {children}
    </button>
  );
}

type Tone = "neutral" | "ok" | "bad" | "warn" | "info" | "accent";
const tones: Record<Tone, string> = {
  neutral: "bg-surface-2 text-muted border-border",
  ok: "bg-ok-soft text-ok border-transparent",
  bad: "bg-bad-soft text-bad border-transparent",
  warn: "bg-warn-soft text-warn border-transparent",
  info: "bg-info-soft text-info border-transparent",
  accent: "bg-accent-soft text-accent border-transparent",
};

export function Badge({ tone = "neutral", children, className, dot }: { tone?: Tone; children: ReactNode; className?: string; dot?: boolean }) {
  return (
    <span className={cx("inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-semibold leading-4 whitespace-nowrap", tones[tone], className)}>
      {dot && <span className="size-1.5 rounded-full bg-current" />}
      {children}
    </span>
  );
}

export function statusTone(s: string | number | undefined): Tone {
  if (typeof s === "number") return s >= 500 ? "bad" : s >= 400 ? "warn" : s >= 300 ? "info" : "ok";
  const v = (s || "").toLowerCase();
  if (["pass", "succeeded", "enabled", "registered", "ok", "healthy", "true"].includes(v)) return "ok";
  if (["fail", "failed", "error", "disabled", "critical"].includes(v)) return "bad";
  if (["missing", "stale", "deleting", "warning", "accepted", "inprogress", "running"].includes(v)) return "warn";
  return "neutral";
}

export function Card({ children, className, title, actions, padded = true }: { children: ReactNode; className?: string; title?: ReactNode; actions?: ReactNode; padded?: boolean }) {
  return (
    <section className={cx("rounded-xl border border-border bg-surface shadow-card", className)}>
      {(title || actions) && (
        <header className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">{title}</h2>
          <div className="flex items-center gap-2">{actions}</div>
        </header>
      )}
      <div className={padded ? "p-4" : ""}>{children}</div>
    </section>
  );
}

export function PageHeader({ title, description, actions, icon }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div className="flex items-start gap-3">
        {icon && <div className="mt-0.5 grid size-10 place-items-center rounded-xl bg-accent-soft text-accent">{icon}</div>}
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight">{title}</h1>
          {description && <p className="mt-0.5 max-w-3xl text-sm text-muted">{description}</p>}
        </div>
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Stat({ label, value, hint, tone, href, icon }: { label: string; value: ReactNode; hint?: ReactNode; tone?: Tone; href?: string; icon?: ReactNode }) {
  const body = (
    <div className="group flex h-full flex-col justify-between rounded-xl border border-border bg-surface p-4 shadow-card transition-colors hover:border-border-strong">
      <div className="flex items-center justify-between text-xs font-medium text-muted">
        <span>{label}</span>
        <span className="text-subtle transition-colors group-hover:text-accent">{icon}</span>
      </div>
      <div className={cx("mt-2 text-[28px] font-semibold tabular-nums tracking-tight", tone === "ok" && "text-ok", tone === "bad" && "text-bad", tone === "warn" && "text-warn")}>{value}</div>
      {hint && <div className="mt-1 text-xs text-subtle">{hint}</div>}
    </div>
  );
  return href ? <Link href={href}>{body}</Link> : body;
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("skeleton h-4", className)} />;
}

export function SkeletonRows({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-3 p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="size-8 rounded-lg" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-1/3" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function EmptyState({ title, description, action, icon }: { title: string; description?: ReactNode; action?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-12 text-center">
      <div className="grid size-11 place-items-center rounded-xl bg-surface-2 text-subtle">{icon ?? <Inbox className="size-5" />}</div>
      <div className="text-sm font-semibold">{title}</div>
      {description && <div className="max-w-sm text-sm text-muted">{description}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

export function ErrorState({ error, retry }: { error: ApiError | Error; retry?: () => void }) {
  const e = error as ApiError;
  const denied = e.status === 403;
  return (
    <div className="m-4 flex items-start gap-3 rounded-lg border border-transparent bg-bad-soft p-3 text-sm text-bad">
      <TriangleAlert className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="font-semibold">
          {e.code || "Error"}
          {e.status ? <span className="ml-2 font-mono text-xs opacity-75">HTTP {e.status}</span> : null}
        </div>
        <div className="mt-0.5 break-words opacity-90">{e.message}</div>
        {denied && <div className="mt-1 text-xs opacity-80">The current identity is not allowed to do this. Switch identity in the top bar to compare.</div>}
      </div>
      {retry && (
        <Button size="sm" variant="ghost" onClick={retry} icon={<RefreshCw className="size-3.5" />}>
          Retry
        </Button>
      )}
    </div>
  );
}

export function CopyButton({ value, label, className }: { value: string; label?: string; className?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      title={label ? `Copy ${label}` : "Copy"}
      onClick={(e) => {
        e.stopPropagation();
        navigator.clipboard?.writeText(value);
        setDone(true);
        setTimeout(() => setDone(false), 1200);
      }}
      className={cx("inline-flex items-center gap-1 rounded-md p-1 text-subtle transition-colors hover:bg-surface-2 hover:text-fg", className)}
    >
      {done ? <Check className="size-3.5 text-ok" /> : <Copy className="size-3.5" />}
      {label && <span className="text-xs">{done ? "Copied" : label}</span>}
    </button>
  );
}

export function Mono({ children, className, copy }: { children: string; className?: string; copy?: boolean }) {
  return (
    <span className={cx("inline-flex min-w-0 items-center gap-1 font-mono text-xs text-muted", className)}>
      <span className="truncate">{children}</span>
      {copy && <CopyButton value={children} />}
    </span>
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded border border-border bg-surface-2 px-1.5 py-px font-mono text-[10px] font-medium text-muted">{children}</kbd>;
}

export function Tabs<T extends string>({ tabs, value, onChange }: { tabs: { id: T; label: string; count?: number; icon?: ReactNode }[]; value: T; onChange: (v: T) => void }) {
  return (
    <div className="flex gap-1 overflow-x-auto border-b border-border" role="tablist">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={value === t.id}
          onClick={() => onChange(t.id)}
          className={cx(
            "relative -mb-px flex items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2.5 text-sm font-medium transition-colors",
            value === t.id ? "border-accent text-fg" : "border-transparent text-muted hover:text-fg",
          )}
        >
          {t.icon}
          {t.label}
          {t.count !== undefined && <span className="rounded-full bg-surface-2 px-1.5 text-[11px] tabular-nums text-muted">{t.count}</span>}
        </button>
      ))}
    </div>
  );
}

const field = "h-9 w-full rounded-lg border border-border bg-surface px-3 text-sm text-fg placeholder:text-subtle transition-colors hover:border-border-strong focus:border-accent focus:outline-none";

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={cx(field, props.className)} />;
}

export function Select({ children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select {...props} className={cx(field, "pr-8", props.className)}>
      {children}
    </select>
  );
}

export function Field({ label, hint, children, error }: { label: string; hint?: ReactNode; children: ReactNode; error?: string }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-semibold text-fg">{label}</span>
      {children}
      {error ? <span className="block text-xs text-bad">{error}</span> : hint ? <span className="block text-xs text-subtle">{hint}</span> : null}
    </label>
  );
}

export function StatusDot({ tone = "ok", pulse }: { tone?: Tone; pulse?: boolean }) {
  const c = { ok: "bg-ok", bad: "bg-bad", warn: "bg-warn", info: "bg-info", neutral: "bg-subtle", accent: "bg-accent" }[tone];
  return (
    <span className="relative inline-flex size-2">
      {pulse && <span className={cx("absolute inset-0 rounded-full opacity-60 animate-ping", c)} />}
      <span className={cx("relative inline-flex size-2 rounded-full", c)} />
    </span>
  );
}

export function Avatar({ name, kind = "user" }: { name: string; kind?: "user" | "group" | "app" | "sp" | "role" }) {
  const initials = (name || "?")
    .replace(/[^A-Za-z0-9 \-_.@]/g, "")
    .split(/[\s\-_.@]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((s) => s[0]!.toUpperCase())
    .join("");
  const palette = {
    user: "bg-accent-soft text-accent",
    group: "bg-ok-soft text-ok",
    app: "bg-info-soft text-info",
    sp: "bg-warn-soft text-warn",
    role: "bg-bad-soft text-bad",
  }[kind];
  return <span className={cx("grid size-8 shrink-0 place-items-center rounded-lg text-xs font-bold", palette, kind === "user" && "rounded-full")}>{initials || "?"}</span>;
}
