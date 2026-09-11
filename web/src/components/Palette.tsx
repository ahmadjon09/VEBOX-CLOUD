import { useState } from "react";
import type { MediaColor } from "../services/media";
import { useI18n } from "../i18n/context";

export function cleanColors(colors?: MediaColor[]): MediaColor[] {
  if (!colors?.length) return [];
  const seen = new Set<string>();
  const out: MediaColor[] = [];
  for (const c of colors) {
    const hex = (c?.hex || "").toLowerCase();
    if (!/^#[0-9a-f]{6}$/.test(hex) || seen.has(hex)) continue;
    seen.add(hex);
    out.push({ hex, pct: Math.max(0, Math.min(100, Number(c.pct) || 0)) });
    if (out.length >= 8) break;
  }
  return out;
}

export function PaletteDots({ colors, size = 10 }: { colors?: MediaColor[]; size?: number }) {
  const list = cleanColors(colors);
  if (list.length < 2) return null;
  return (
    <span className="flex items-center gap-0.5" aria-label="palette">
      {list.map((c) => (
        <span
          key={c.hex}
          title={`${c.hex} · ${c.pct.toFixed(1)}%`}
          className="border border-line2"
          style={{ width: size, height: size, backgroundColor: c.hex, display: "inline-block" }}
        />
      ))}
    </span>
  );
}

export function PaletteBars({ colors }: { colors?: MediaColor[] }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState("");
  const list = cleanColors(colors);
  if (!list.length) return null;

  const copy = async (hex: string) => {
    try {
      await navigator.clipboard.writeText(hex);
      setCopied(hex);
      setTimeout(() => setCopied(""), 1500);
    } catch {
      setCopied("");
    }
  };

  return (
    <div className="mt-4 border border-line bg-ink px-4 py-3">
      <div className="micro mb-3 text-dim">{t("md.colors")}</div>
      <div className="space-y-2">
        {list.map((c) => (
          <button
            key={c.hex}
            type="button"
            onClick={() => copy(c.hex)}
            title={copied ? t("md.colors.copied") : t("md.colors.copy")}
            className="flex w-full items-center gap-3 text-left"
          >
            <span className="h-5 w-5 shrink-0 border border-line2" style={{ backgroundColor: c.hex }} />
            <span className="w-[74px] shrink-0 text-[11px] uppercase text-fg">{c.hex}</span>
            <span className="h-1.5 flex-1 bg-panel2">
              <span className="block h-full bg-fg" style={{ width: `${Math.max(2, c.pct)}%` }} />
            </span>
            <span className="w-[46px] shrink-0 text-right text-[10px] text-mut">{c.pct.toFixed(1)}%</span>
          </button>
        ))}
      </div>
    </div>
  );
}
