import { useState } from "react";
import { Link } from "react-router-dom";
import { Check, Eye, FileAudio, Globe, Lock, Play, TriangleAlert } from "lucide-react";
import { deliveryUrl, type MediaRow, type UploadProgress } from "../services/media";
import { bytes, durFmt } from "../utils/format";
import { PaletteDots } from "./Palette";

function tileBg(bg?: string): string | undefined {
  return bg && /^#[0-9a-fA-F]{6}$/.test(bg) ? bg : undefined;
}

export function audioLabel(item: MediaRow): string {
  if (item.mime_type === "audio/mpeg") return "MP3";
  if (item.mime_type === "audio/mp4") return "M4A";
  const dot = item.filename.lastIndexOf(".");
  if (dot > 0) return item.filename.slice(dot + 1).toUpperCase().slice(0, 5);
  return "AUDIO";
}

export function MediaCard({
  item,
  selected,
  selectable,
  onToggle,
}: {
  item: MediaRow;
  selected?: boolean;
  selectable?: boolean;
  onToggle?: (id: string) => void;
}) {
  const [loaded, setLoaded] = useState(false);
  const isAudio = item.kind === "audio";

  return (
    <div
      className={`group relative border bg-panel transition-colors ${
        selected ? "border-fg" : "border-line hover:border-line2"
      }`}
    >
      <Link to={`/media/${item.id}`} className="block">
        {isAudio ? (
          <div className="group relative flex aspect-square flex-col items-center justify-center gap-3 bg-panel2 p-4">
            <FileAudio size={30} className="text-mut" aria-hidden />
            <span className="micro text-mut">{audioLabel(item)}</span>
            {item.duration_fmt && <span className="micro text-dim">{item.duration_fmt}</span>}
            <span className="absolute inset-0 flex items-center justify-center bg-ink/0 transition-colors group-hover:bg-ink/40">
              <span className="flex h-10 w-10 scale-75 items-center justify-center rounded-full border border-fg bg-ink/80 text-fg opacity-0 transition-all group-hover:scale-100 group-hover:opacity-100">
                <Play size={15} aria-hidden />
              </span>
            </span>
            <div className="flex w-full items-end justify-center gap-0.5" aria-hidden>
              {[3, 6, 4, 8, 5, 9, 4, 7, 3, 6, 5, 8, 4, 3, 6, 5].map((h, i) => (
                <span key={i} className="w-1 bg-line2" style={{ height: `${h * 2}px` }} />
              ))}
            </div>
          </div>
        ) : (
          <div
            className="relative w-full overflow-hidden"
            style={{
              aspectRatio: item.width && item.height ? `${item.width}/${item.height}` : "4/3",
              backgroundColor: tileBg(item.bg) || "var(--color-panel2)",
            }}
          >
            <img
              src={deliveryUrl(item)}
              alt={item.filename}
              loading="lazy"
              decoding="async"
              onLoad={() => setLoaded(true)}
              className={`media-fade absolute inset-0 h-full w-full object-cover ${loaded ? "is-loaded" : ""}`}
            />
          </div>
        )}
      </Link>

      {selectable && (
        <button
          onClick={() => onToggle?.(item.id)}
          aria-label="select"
          className={`absolute right-2 top-2 flex h-6 w-6 items-center justify-center border transition-colors ${
            selected ? "border-fg bg-fg text-ink" : "border-line2 bg-ink/70 text-transparent hover:text-mut"
          }`}
        >
          <Check size={12} aria-hidden />
        </button>
      )}

      <span
        title={item.visibility}
        className={`absolute left-2 top-2 flex items-center gap-1 border px-1.5 py-0.5 ${
          item.visibility === "public" ? "border-ok/40 bg-ink/70 text-ok" : "border-line2 bg-ink/70 text-mut"
        }`}
      >
        {item.visibility === "public" ? <Globe size={10} /> : <Lock size={10} />}
      </span>

      <div className="space-y-1 border-t border-line px-2.5 py-2">
        <div className="truncate text-xs text-fg" title={item.filename}>
          {item.ready ? item.filename : <span className="text-dim">{item.id}</span>}
        </div>
        <div className="flex items-center justify-between gap-2 text-[10px] text-mut">
          <span className="flex items-center gap-1.5">
            <span className={item.ready ? "" : "opacity-40"}>{item.ready ? bytes(item.size_bytes) : "0 B"}</span>
            <PaletteDots colors={item.colors} />
          </span>
          <span className="flex items-center gap-2">
            {item.ready && item.views > 0 && (
              <span className="flex items-center gap-1">
                <Eye size={10} aria-hidden /> {item.views}
              </span>
            )}
            {item.duration_fmt && <span>{durFmt(item.duration)}</span>}
          </span>
        </div>
      </div>
    </div>
  );
}

export function UploadCard({ p }: { p: UploadProgress }) {
  return (
    <div className={`border bg-panel ${p.state === "error" ? "border-err/50" : "border-line"}`}>
      <div className="flex items-center justify-between gap-2 px-2.5 py-2">
        <span className="truncate text-xs text-fg">{p.name}</span>
        <span
          className={`micro flex items-center gap-1 ${
            p.state === "error" ? "text-err" : p.state === "done" ? "text-ok" : "text-mut"
          }`}
        >
          {p.state === "done" && <Check size={11} aria-hidden />}
          {p.state === "error" && <TriangleAlert size={11} aria-hidden />}
          {p.state === "done" ? "OK" : p.state === "error" ? "ERR" : `${p.pct}%`}
        </span>
      </div>
      {p.state === "uploading" && (
        <div className="h-0.5 w-full bg-panel2">
          <div className="h-full bg-fg transition-all" style={{ width: `${p.pct}%` }} />
        </div>
      )}
      {p.error && <div className="px-2.5 pb-2 text-[10px] text-err">{p.error}</div>}
    </div>
  );
}

export function MediaGrid({
  items,
  uploads,
  selectable = false,
  selectedIds,
  onToggle,
}: {
  items: MediaRow[];
  uploads: UploadProgress[];
  selectable?: boolean;
  selectedIds: Set<string>;
  onToggle: (id: string) => void;
}) {
  return (
    <div className="masonry">
      {uploads.map((u) => (
        <UploadCard key={u.id} p={u} />
      ))}
      {items.map((it) => (
        <MediaCard
          key={it.id}
          item={it}
          selectable={selectable}
          selected={selectedIds.has(it.id)}
          onToggle={onToggle}
        />
      ))}
    </div>
  );
}
