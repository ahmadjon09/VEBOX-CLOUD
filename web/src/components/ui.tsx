
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import {
  CircleAlert,
  CircleCheck,
  CircleX,
  Copy as CopyIcon,
  FolderOpen,
  RefreshCw,
  X,
} from "lucide-react";
import { useI18n } from "../i18n/context";
import { LANGS, type Lang } from "../i18n/translations";

export function LangSwitcher() {
  const { lang, setLang } = useI18n();
  return (
    <div className="flex border border-line bg-ink" role="group" aria-label="Language">
      {LANGS.map((l) => (
        <button
          key={l.code}
          onClick={() => setLang(l.code as Lang)}
          aria-pressed={lang === l.code}
          aria-label={l.code}
          className={`micro px-2 py-1 ${lang === l.code ? "bg-accent text-white" : "text-mut hover:text-fg"}`}
        >
          {l.label}
        </button>
      ))}
    </div>
  );
}

interface Toast {
  id: number;
  text: string;
  kind: "ok" | "err";
}

const ToastCtx = createContext<(text: string, kind?: "ok" | "err") => void>(() => {});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);

  const push = useCallback((text: string, kind: "ok" | "err" = "ok") => {
    const id = Date.now() + Math.random();
    setItems((xs) => [...xs, { id, text, kind }]);
    setTimeout(() => setItems((xs) => xs.filter((x) => x.id !== id)), 3500);
  }, []);

  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed left-1/2 top-3 z-[100] -translate-x-1/2 space-y-2" role="status" aria-live="polite">
        {items.map((t) => (
          <div
            key={t.id}
            className={`micro flex items-center gap-2 border px-4 py-2.5 bg-panel2 shadow-lg ${
              t.kind === "ok" ? "border-ok/40 text-ok" : "border-err/40 text-err"
            }`}
          >
            {t.kind === "ok" ? <CircleCheck size={13} aria-hidden /> : <CircleX size={13} aria-hidden />}
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast() {
  return useContext(ToastCtx);
}

export function Micro({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`micro text-mut ${className}`}>{children}</div>;
}

type BtnProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "solid" | "ghost" | "danger";
  loading?: boolean;
};

export function Btn({ variant = "ghost", loading, className = "", children, disabled, ...rest }: BtnProps) {
  const base =
    "micro inline-flex items-center justify-center gap-2 border px-4 py-3 transition-colors disabled:opacity-40 disabled:pointer-events-none active:translate-y-px";
  const v =
    variant === "solid"
      ? "border-fg bg-fg text-ink hover:bg-mut hover:border-mut"
      : variant === "danger"
      ? "border-err/40 text-err hover:bg-err/10 hover:border-err"
      : "border-line text-fg hover:border-line2 hover:bg-panel2";
  return (
    <button className={`${base} ${v} ${className}`} disabled={disabled || loading} {...rest}>
      {loading && <SpinnerInline />}
      {children}
    </button>
  );
}

function SpinnerInline() {
  return <RefreshCw size={12} className="animate-spin" aria-hidden />;
}

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`w-full border border-line bg-panel px-3.5 py-3 text-sm text-fg placeholder:text-dim focus:border-line2 focus:outline-none ${props.className || ""}`}
    />
  );
}

function pageList(page: number, pageCount: number): (number | "…")[] {
  if (pageCount <= 7) {
    return Array.from({ length: pageCount }, (_, i) => i + 1);
  }
  const out: (number | "…")[] = [1];
  const lo = Math.max(2, page - 1);
  const hi = Math.min(pageCount - 1, page + 1);
  if (lo > 2) out.push("…");
  for (let p = lo; p <= hi; p++) out.push(p);
  if (hi < pageCount - 1) out.push("…");
  out.push(pageCount);
  return out;
}

export function Pager({
  page,
  pageCount,
  onChange,
}: {
  page: number;
  pageCount: number;
  onChange: (page: number) => void;
}) {
  if (pageCount <= 1) return null;
  const go = (p: number) => {
    if (p >= 1 && p <= pageCount && p !== page) onChange(p);
  };
  return (
    <nav className="flex flex-wrap items-center justify-center gap-1" aria-label="Pagination">
      <button
        onClick={() => go(page - 1)}
        disabled={page <= 1}
        aria-label="Oldingi sahifa"
        className="micro flex h-8 w-8 items-center justify-center border border-line text-mut transition-colors hover:border-line2 hover:text-fg disabled:pointer-events-none disabled:opacity-30"
      >
        ‹
      </button>
      {pageList(page, pageCount).map((p, i) =>
        p === "…" ? (
          <span key={`e${i}`} className="micro px-1.5 text-dim">
            …
          </span>
        ) : (
          <button
            key={p}
            onClick={() => go(p)}
            aria-current={p === page ? "page" : undefined}
            className={`micro h-8 min-w-8 px-2 border transition-colors ${
              p === page
                ? "border-fg bg-fg text-ink"
                : "border-line text-mut hover:border-line2 hover:text-fg"
            }`}
          >
            {p}
          </button>
        )
      )}
      <button
        onClick={() => go(page + 1)}
        disabled={page >= pageCount}
        aria-label="Keyingi sahifa"
        className="micro flex h-8 w-8 items-center justify-center border border-line text-mut transition-colors hover:border-line2 hover:text-fg disabled:pointer-events-none disabled:opacity-30"
      >
        ›
      </button>
    </nav>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <Micro className="mb-2">{label}</Micro>
      {children}
    </label>
  );
}

export function Badge({
  children,
  tone = "mut",
}: {
  children: ReactNode;
  tone?: "mut" | "ok" | "warn" | "err" | "fg";
}) {
  const c = {
    mut: "border-line text-mut",
    ok: "border-ok/40 text-ok",
    warn: "border-warn/40 text-warn",
    err: "border-err/40 text-err",
    fg: "border-line2 text-fg",
  }[tone];
  return <span className={`micro inline-flex items-center gap-1.5 border px-2.5 py-1.5 ${c}`}>{children}</span>;
}

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-3 py-16" role="status">
      <RefreshCw size={15} className="animate-spin text-mut" aria-hidden />
      {label && <Micro>{label}</Micro>}
    </div>
  );
}

export function ErrorBox({ text, onRetry }: { text: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center gap-4 py-16">
      <Micro className="flex items-center gap-2 text-err">
        <CircleAlert size={14} aria-hidden /> {text}
      </Micro>
      {onRetry && (
        <Btn onClick={onRetry} className="px-6">
          <RefreshCw size={12} aria-hidden /> Retry
        </Btn>
      )}
    </div>
  );
}

export function Empty({ title, desc, action }: { title: string; desc?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 border border-dashed border-line px-6 py-20 text-center">
      <FolderOpen size={28} className="text-dim" aria-hidden />
      <div className="micro text-fg">{title}</div>
      {desc && <p className="max-w-md text-xs leading-relaxed text-mut">{desc}</p>}
      {action}
    </div>
  );
}

export function Modal({
  title,
  children,
  onClose,
  wide = false,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
}) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", h);
      document.body.style.overflow = "";
    };
  }, [onClose]);

  return (
    <div
      data-vb-modal
      className="fixed inset-0 z-[90] flex items-end justify-center bg-ink/80 backdrop-blur-sm sm:items-center sm:p-6"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        className={`w-full border border-line2 bg-panel sm:border ${wide ? "sm:max-w-2xl" : "sm:max-w-md"} max-h-[88vh] overflow-y-auto`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="flex items-center justify-between border-b border-line px-5 py-4">
          <div className="micro text-fg">{title}</div>
          <button
            onClick={onClose}
            className="flex h-7 w-7 items-center justify-center text-mut hover:text-fg"
            aria-label="Close"
          >
            <X size={14} />
          </button>
        </div>
        <div className="px-5 py-5">{children}</div>
      </div>
    </div>
  );
}

export function ConfirmBody({
  desc,
  confirmLabel,
  busy,
  onConfirm,
  onCancel,
}: {
  desc: string;
  confirmLabel: string;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="space-y-5">
      <p className="text-sm leading-relaxed text-mut">{desc}</p>
      <div className="flex gap-3">
        <Btn onClick={onCancel} className="flex-1">
          Cancel
        </Btn>
        <Btn variant="danger" onClick={onConfirm} loading={busy} className="flex-1">
          {confirmLabel}
        </Btn>
      </div>
    </div>
  );
}

export function Stat({ k, v }: { k: string; v: string }) {
  return (
    <div className="border border-line bg-panel px-4 py-4">
      <Micro>{k}</Micro>
      <div className="mt-2 text-lg text-fg">{v}</div>
    </div>
  );
}

export function Copy({ text, label }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  const toast = useToast();
  return (
    <button
      onClick={async () => {
        if (await copyText(text)) {
          setDone(true);
          toast(label || "Copied");
          setTimeout(() => setDone(false), 1500);
        }
      }}
      aria-label={label || "Copy"}
      className={`micro shrink-0 flex items-center gap-1.5 border px-2.5 py-1.5 transition-colors ${
        done ? "border-ok/40 text-ok" : "border-line text-mut hover:border-line2 hover:text-fg"
      }`}
    >
      {done ? <CircleCheck size={12} aria-hidden /> : <CopyIcon size={12} aria-hidden />}
      {done ? "OK" : null}
    </button>
  );
}

export async function copyText(s: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(s);
    return true;
  } catch {
    try {
      const ta = document.createElement("textarea");
      ta.value = s;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
      return true;
    } catch {
      return false;
    }
  }
}
