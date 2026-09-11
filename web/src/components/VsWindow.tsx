
import type { ReactNode } from "react";
import { X } from "lucide-react";
import { Logo } from "./Logo";

export function VsWindow({
  title,
  children,
  className = "",
  toolbar,
}: {
  title: string;
  children: ReactNode;
  className?: string;
  toolbar?: ReactNode;
}) {
  return (
    <div className={`overflow-hidden border border-line bg-ink ${className}`}>
      <div className="flex items-center gap-2 border-b border-line bg-titlebar px-3 py-2">
        <div className="flex items-center gap-1.5" aria-hidden>
          <span className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[#28c840]" />
        </div>
        <span className="micro ml-2 text-mut">{title}</span>
      </div>
      {toolbar}
      <div>{children}</div>
    </div>
  );
}

function fileDot(name: string): string {
  if (name.endsWith(".tsx") || name.endsWith(".ts")) return "#519aba";
  if (name.endsWith(".json")) return "#cbcb41";
  if (name.startsWith("terminal")) return "#89d185";
  if (name.startsWith("status.")) return "#89d185";
  return "#858585";
}

export function VsTabs({
  tabs,
  active = 0,
  onSelect,
}: {
  tabs: string[];
  active?: number;
  onSelect?: (i: number) => void;
}) {
  const tabCls = (i: number) =>
    `tab-item flex items-center gap-2 border-r border-line px-4 py-2 text-xs transition-colors ${
      i === active
        ? "bg-ink text-fg shadow-[inset_0_2px_0_0_var(--color-accent)]"
        : "bg-panel text-mut hover:bg-panel2 hover:text-fg"
    }`;
  const inner = (t: string, i: number) => (
    <>
      <span className="h-1.5 w-1.5 rounded-full" style={{ background: fileDot(t) }} aria-hidden />
      {t}
      {i === active && <X size={11} className="text-dim" aria-hidden />}
    </>
  );
  return (
    <div className="flex border-b border-line bg-panel" role={onSelect ? "tablist" : undefined}>
      {tabs.map((t, i) =>
        onSelect ? (
          <button
            key={t}
            type="button"
            role="tab"
            aria-selected={i === active}
            onClick={() => onSelect(i)}
            className={tabCls(i)}
          >
            {inner(t, i)}
          </button>
        ) : (
          <div key={t} className={tabCls(i)}>
            {inner(t, i)}
          </div>
        )
      )}
    </div>
  );
}

export function VsMiniStatus() {
  return (
    <div className="flex items-center justify-between bg-accent px-3 py-1 text-[10px] text-white">
      <span className="flex items-center gap-1.5">
        <Logo size={10} /> vebox
      </span>
      <span>UTF-8 · LF · v2.0.0</span>
    </div>
  );
}

export function CodeBlock({ lines, className = "" }: { lines: ReactNode[]; className?: string }) {
  return (
    <div className={`overflow-x-auto text-[12.5px] leading-6 ${className}`}>
      <table className="w-full border-collapse">
        <tbody>
          {lines.map((l, i) => (
            <tr key={i} className="hover:bg-panel/60">
              <td className="w-10 select-none pr-4 text-right align-top text-dim">{i + 1}</td>
              <td className="whitespace-pre pr-4">{l}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
