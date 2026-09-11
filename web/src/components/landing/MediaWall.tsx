
import { useEffect, useState } from "react";
import { Check, FileAudio, Link2 } from "lucide-react";
import { VsTabs, VsWindow, VsMiniStatus } from "../VsWindow";
import { bytes } from "../../utils/format";

const SAMPLES = [
  { img: "/samples/art-01.jpg", name: "brand-01.jpg", vis: "PUBLIC" as const },
  { img: "/samples/product-01.jpg", name: "drop-01.jpg", vis: "PUBLIC" as const },
  { img: "/samples/cover-01.jpg", name: "ep12-cover.jpg", vis: "PUBLIC" as const },
  { img: "/samples/art-02.jpg", name: "wall-02.jpg", vis: "PRIVATE" as const },
];

const AUDIO_ROWS = ["podcast-ep12.m4a", "mix-07.mp3"];

type Filter = "ALL" | "IMG" | "AUDIO" | "PRIVATE";

export function MediaWall() {
  const [sizes, setSizes] = useState<Record<string, number>>({});
  const [filter, setFilter] = useState<Filter>("ALL");

  useEffect(() => {
    let alive = true;
    (async () => {
      const out: Record<string, number> = {};
      for (const s of SAMPLES) {
        try {
          const r = await fetch(s.img, { method: "HEAD" });
          const n = Number(r.headers.get("content-length") || 0);
          if (n) out[s.name] = n;
        } catch {
        }
      }
      if (alive) setSizes(out);
    })();
    return () => {
      alive = false;
    };
  }, []);

  const rows = SAMPLES.filter((f) =>
    filter === "ALL" ? true : filter === "PRIVATE" ? f.vis === "PRIVATE" : f.vis === "PUBLIC"
  );
  const showAudio = filter === "ALL" || filter === "AUDIO";
  const shown = [...rows, ...(showAudio ? AUDIO_ROWS.map((name) => ({ name, size: 0 })) : [])];
  const total = shown.reduce((a, r) => a + (sizes[r.name] || 0), 0);

  return (
    <VsWindow title="files.tsx — vebox">
      <VsTabs tabs={["files.tsx", "analytics.tsx", "keys.tsx"]} active={0} />
      <div className="flex items-center gap-1.5 border-b border-line bg-panel px-3 py-2">
        {(["ALL", "IMG", "AUDIO", "PRIVATE"] as Filter[]).map((f) => (
          <button
            key={f}
            type="button"
            aria-pressed={filter === f}
            onClick={() => setFilter(f)}
            className={`micro px-2 py-0.5 transition-colors ${
              filter === f ? "bg-accent text-white" : "bg-panel2 text-mut hover:text-fg"
            }`}
          >
            {f}
          </button>
        ))}
        <span className="micro ml-auto text-dim">
          {shown.length} files{total ? ` · ${bytes(total)}` : ""}
        </span>
      </div>
      <div className={`grid gap-1.5 p-3 ${rows.length ? "grid-cols-2 sm:grid-cols-4" : "grid-cols-1"}`}>
        {rows.map((f) => (
          <div
            key={f.name}
            className="group overflow-hidden border border-line bg-ink transition-colors hover:border-line2"
          >
            <div className="relative" style={{ aspectRatio: "4/3" }}>
              <img
                src={f.img}
                alt={f.name}
                loading="lazy"
                className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.05]"
              />
              <span
                className={`micro absolute left-1 top-1 px-1 py-px ${
                  f.vis === "PUBLIC" ? "bg-ink/80 text-ok" : "bg-ink/80 text-warn"
                }`}
              >
                {f.vis}
              </span>
              <div className="pointer-events-none absolute inset-0 flex items-end justify-end bg-gradient-to-t from-ink/70 via-transparent to-transparent opacity-0 transition-opacity duration-200 group-hover:opacity-100">
                <span className="px-1.5 py-1 text-fg">
                  <Link2 size={11} aria-hidden />
                </span>
              </div>
            </div>
            <div className="flex items-center justify-between px-1.5 py-1 text-[9px]">
              <span className="truncate text-mut">{f.name}</span>
              <span className="text-dim">{sizes[f.name] ? bytes(sizes[f.name]) : "—"}</span>
            </div>
          </div>
        ))}
        {rows.length === 0 && (
          <div className="micro px-1 py-6 text-center text-dim">
            {filter === "AUDIO" ? "0 images · faqat audio" : "bu filtr bo'sh"}
          </div>
        )}
      </div>
      {showAudio && (
        <div className="mx-3 mb-3 space-y-1">
          {AUDIO_ROWS.map((name) => (
            <div key={name} className="flex items-center gap-2 border border-line bg-panel px-2 py-1.5">
              <FileAudio size={11} className="text-teal" aria-hidden />
              <span className="text-[10px] text-mut">{name}</span>
              <span className="ml-auto micro flex items-center gap-1 text-ok">
                <Check size={11} aria-hidden /> 100%
              </span>
            </div>
          ))}
        </div>
      )}
      <VsMiniStatus />
    </VsWindow>
  );
}
