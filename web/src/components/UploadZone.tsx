
import { useRef, useState } from "react";
import { ClipboardPaste, CloudUpload } from "lucide-react";
import { useI18n } from "../i18n/context";

export function pickFiles(list: FileList | File[] | null | undefined): File[] {
  if (!list) return [];
  return Array.from(list);
}

const ACCEPT =
  "image/jpeg,image/png,image/webp,image/bmp,image/tiff,image/x-icon,image/svg+xml,image/avif,image/heic,audio/x-m4a,audio/mp4,audio/mpeg,audio/mp3,audio/x-mpeg,.m4a,.mp3";

export function UploadZone({ onFiles }: { onFiles: (files: File[]) => void }) {
  const [over, setOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const { t } = useI18n();

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        onFiles(pickFiles(e.dataTransfer.files));
      }}
      onClick={() => inputRef.current?.click()}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && inputRef.current?.click()}
      className={`flex cursor-pointer flex-col items-center justify-center gap-3 border border-dashed px-6 py-14 transition-colors ${
        over ? "border-fg bg-panel2" : "border-line hover:border-line2"
      }`}
    >
      <input
        ref={inputRef}
        type="file"
        multiple
        accept={ACCEPT}
        className="hidden"
        onChange={(e) => {
          onFiles(pickFiles(e.target.files));
          e.target.value = "";
        }}
      />
      <CloudUpload size={26} className="text-mut" aria-hidden />
      <div className="micro text-fg">{t("con.drop")}</div>
      <div className="text-[11px] text-mut">{t("con.drop.hint")}</div>
      <div className="flex items-center gap-1.5 text-[10px] text-dim">
        <ClipboardPaste size={10} aria-hidden /> {t("con.paste")}
      </div>
    </div>
  );
}
