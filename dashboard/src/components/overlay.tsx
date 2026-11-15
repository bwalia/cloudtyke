"use client";

import { CheckCircle2, Info, X, XCircle } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { ApiError } from "@/lib/api";
import { Button, cx } from "./ui";

function useEscape(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [open, onClose]);
}

function Portal({ children }: { children: ReactNode }) {
  return typeof document === "undefined" ? null : createPortal(children, document.body);
}

/** Side panel for details: keeps the list in view, unlike Azure's full-page blades. */
export function Drawer({
  open,
  onClose,
  title,
  subtitle,
  icon,
  actions,
  children,
  wide,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  subtitle?: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  wide?: boolean;
}) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <Portal>
      <div className="fixed inset-0 z-40 bg-black/25 backdrop-blur-[1px] animate-in" onClick={onClose} />
      <aside
        role="dialog"
        aria-modal="true"
        className={cx("fixed inset-y-0 right-0 z-50 flex w-full flex-col border-l border-border bg-surface shadow-pop animate-slide", wide ? "max-w-3xl" : "max-w-xl")}
      >
        <header className="flex items-start gap-3 border-b border-border px-5 py-4">
          {icon}
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-base font-semibold">{title}</h2>
            {subtitle && <div className="mt-0.5 truncate text-xs text-muted">{subtitle}</div>}
          </div>
          <div className="flex items-center gap-1">
            {actions}
            <button onClick={onClose} className="rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-fg" aria-label="Close">
              <X className="size-4" />
            </button>
          </div>
        </header>
        <div className="flex-1 overflow-y-auto">{children}</div>
      </aside>
    </Portal>
  );
}

export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
}) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <Portal>
      <div className="fixed inset-0 z-50 grid place-items-center bg-black/35 p-4 backdrop-blur-[2px] animate-in" onMouseDown={onClose}>
        <div role="dialog" aria-modal="true" className="w-full max-w-lg rounded-2xl border border-border bg-surface shadow-pop" onMouseDown={(e) => e.stopPropagation()}>
          <div className="px-5 pt-5">
            <h2 className="text-base font-semibold">{title}</h2>
            {description && <p className="mt-1 text-sm text-muted">{description}</p>}
          </div>
          {children && <div className="space-y-4 px-5 py-4">{children}</div>}
          {footer && <div className="flex justify-end gap-2 border-t border-border bg-surface-2/50 px-5 py-3 rounded-b-2xl">{footer}</div>}
        </div>
      </div>
    </Portal>
  );
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = "Delete",
  typeToConfirm,
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => Promise<unknown> | void;
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  typeToConfirm?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [typed, setTyped] = useState("");
  const ok = !typeToConfirm || typed === typeToConfirm;
  return (
    <Dialog
      open={open}
      onClose={() => {
        setTyped("");
        onClose();
      }}
      title={title}
      description={description}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="danger"
            loading={busy}
            disabled={!ok}
            onClick={async () => {
              setBusy(true);
              try {
                await onConfirm();
                setTyped("");
                onClose();
              } finally {
                setBusy(false);
              }
            }}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {typeToConfirm && (
        <label className="block space-y-1.5 text-sm">
          <span className="text-muted">
            Type <b className="font-mono text-fg">{typeToConfirm}</b> to confirm
          </span>
          <input
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            className="h-9 w-full rounded-lg border border-border bg-surface px-3 font-mono text-sm focus:border-bad focus:outline-none"
          />
        </label>
      )}
    </Dialog>
  );
}

// ------------------------------------------------------------------ toasts

type Toast = { id: number; tone: "ok" | "bad" | "info"; title: string; body?: string };
const ToastCtx = createContext<{ push: (t: Omit<Toast, "id">) => void; error: (e: unknown, title?: string) => void }>({
  push: () => {},
  error: () => {},
});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const seq = useRef(0);
  const push = useCallback((t: Omit<Toast, "id">) => {
    const id = ++seq.current;
    setToasts((ts) => [...ts.slice(-3), { ...t, id }]);
    setTimeout(() => setToasts((ts) => ts.filter((x) => x.id !== id)), t.tone === "bad" ? 9000 : 4500);
  }, []);
  const error = useCallback(
    (e: unknown, title?: string) => {
      const a = e instanceof ApiError ? e : null;
      push({
        tone: "bad",
        title: title ?? (a ? `${a.code}${a.status ? ` · HTTP ${a.status}` : ""}` : "Something went wrong"),
        body: e instanceof Error ? e.message : String(e),
      });
    },
    [push],
  );
  return (
    <ToastCtx.Provider value={{ push, error }}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-[380px] max-w-[calc(100vw-2rem)] flex-col gap-2" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className="pointer-events-auto flex items-start gap-3 rounded-xl border border-border bg-surface p-3 shadow-pop animate-slide">
            {t.tone === "ok" ? <CheckCircle2 className="mt-0.5 size-4 text-ok" /> : t.tone === "bad" ? <XCircle className="mt-0.5 size-4 text-bad" /> : <Info className="mt-0.5 size-4 text-info" />}
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold">{t.title}</div>
              {t.body && <div className="mt-0.5 break-words text-xs text-muted">{t.body}</div>}
            </div>
            <button className="text-subtle hover:text-fg" onClick={() => setToasts((ts) => ts.filter((x) => x.id !== t.id))} aria-label="Dismiss">
              <X className="size-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export const useToast = () => useContext(ToastCtx);
