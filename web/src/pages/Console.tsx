
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Globe,
  Keyboard,
  ListChecks,
  LockOpen,
  Plus,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { useI18n } from "../i18n/context";
import { useHotkeys } from "../hooks/useHotkeys";
import { Seo } from "../components/Seo";
import { MediaGrid } from "../components/MediaGrid";
import { UploadZone } from "../components/UploadZone";
import { Btn, Empty, ErrorBox, Micro, Pager, Spinner, useToast } from "../components/ui";
import {
  bulkDelete,
  bulkVisibility,
  quotaFromError,
  useAccount,
  useMediaPage,
  uploadFile,
  type MediaRow,
  type SortKey,
  type UploadProgress,
} from "../services/media";
import { bytes, kb } from "../utils/format";
import { assetUrl } from "../services/api";

const MAX_FILE = 50 * 1024 * 1024;
const PAGE = 60;
const ACCEPT = [
  "image/jpeg",
  "image/png",
  "image/webp",
  "image/bmp",
  "image/tiff",
  "image/x-icon",
  "image/svg+xml",
  "image/avif",
  "image/heic",
  "audio/x-m4a",
  "audio/mp4",
  "audio/mpeg",
  "audio/mp3",
  "audio/x-mpeg",
];
const AUDIO_MIME = new Set(["audio/x-m4a", "audio/mp4", "audio/mpeg", "audio/mp3", "audio/x-mpeg", "audio/mpeg3"]);

type VisFilter = "" | "public" | "private";
type KindFilter = "" | "image" | "audio";

export function Console() {
  const { t, lang } = useI18n();
  const toast = useToast();
  const nav = useNavigate();

  const [kind, setKind] = useState<KindFilter>("");
  const [vis, setVis] = useState<VisFilter>("");
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<SortKey>("newest");
  const [offset, setOffset] = useState(0);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [uploads, setUploads] = useState<UploadProgress[]>([]);
  const [showUpload, setShowUpload] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);
  const qRef = useRef(q);
  qRef.current = q;
  const searchRef = useRef<HTMLInputElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useHotkeys({
    "/": () => searchRef.current?.focus(),
    u: () => fileRef.current?.click(),
    U: () => fileRef.current?.click(),
    "?": () => nav("/console/help"),
  });

  const [qLive, setQLive] = useState("");
  useEffect(() => {
    const id = setTimeout(() => setQ(qRef.current), 350);
    return () => clearTimeout(id);
  }, [q]);

  const { rows: items, total, error, isLoading, mutate } = useMediaPage(
    kind || undefined,
    vis || undefined,
    qLive || undefined,
    sort,
    offset
  );
  const { data: account } = useAccount();

  const refresh = useCallback(() => mutate(), [mutate]);

  const runUpload = useCallback(
    async (files: File[]) => {
      for (const f of files) {
        const name = f.name.toLowerCase();
        const isAudio = AUDIO_MIME.has(f.type) || name.endsWith(".m4a") || name.endsWith(".mp3");
        const okType = ACCEPT.includes(f.type) || isAudio;
        const localId = `up_${Date.now()}_${Math.random().toString(36).slice(2)}`;

        if (!okType || f.size > MAX_FILE) {
          setUploads((xs) => [
            ...xs,
            {
              id: localId,
              name: f.name,
              pct: 0,
              state: "error",
              error: !okType ? "415" : bytes(f.size),
            },
          ]);
          setTimeout(() => setUploads((xs) => xs.filter((x) => x.id !== localId)), 5000);
          continue;
        }

        setUploads((xs) => [...xs, { id: localId, name: f.name, pct: 0, state: "uploading" }]);
        try {
          await uploadFile(f, (pct) =>
            setUploads((xs) => xs.map((x) => (x.id === localId ? { ...x, pct } : x)))
          );
          setUploads((xs) => xs.map((x) => (x.id === localId ? { ...x, pct: 100, state: "done" } : x)));
          refresh();
        } catch (e: any) {
          const quota = quotaFromError(e);
          const msg = quota
            ? `${kb(quota.used)} / ${kb(quota.limit)}`
            : e?.response?.data?.error?.message || e?.response?.status || "ERR";
          setUploads((xs) => xs.map((x) => (x.id === localId ? { ...x, state: "error", error: String(msg) } : x)));
          toast(String(msg), "err");
        }
        setTimeout(() => setUploads((xs) => xs.filter((x) => x.id !== localId)), 2500);
      }
    },
    [refresh, toast]
  );

  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      const fs = Array.from(e.clipboardData?.files || []);
      if (fs.length) {
        e.preventDefault();
        runUpload(fs);
      }
    };
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, [runUpload]);

  const pickDirect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const fs = Array.from(e.target.files || []);
    if (fs.length) runUpload(fs);
    e.target.value = "";
  };

  const toggle = (id: string) =>
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  const allOnPage = items.length > 0 && items.every((i) => selected.has(i.id));
  const toggleAll = () =>
    setSelected(allOnPage ? new Set() : new Set(items.map((i) => i.id)));

  const doBulk = async (fn: () => Promise<void>) => {
    setBulkBusy(true);
    try {
      await fn();
      toast(t("toast.deleted"));
      setSelected(new Set());
      setSelectMode(false);
      refresh();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setBulkBusy(false);
    }
  };

  const usage = account?.data?.account?.usage;
  const quotaBytes = usage ? (usage.storage_quota_bytes || 0) : 0;

  return (
    <div className="mx-auto max-w-[2000px] px-4 py-8 sm:px-6">
      <Seo title={`${t("nav.console")} — VEBOX`} path="/console" />

      <input ref={fileRef} type="file" multiple accept={ACCEPT.join(",") + ",audio/mpeg,.m4a,.mp3"} className="hidden" onChange={pickDirect} />

      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl tracking-tight text-fg">{t("con.title")}</h1>
          {usage && (
            <div className="micro mt-2 text-dim">
              {quotaBytes > 0
                ? `${bytes(usage.storage_bytes)} / ${bytes(quotaBytes)} · ${usage.file_count}`
                : `${bytes(usage.storage_bytes)} · ${usage.file_count}`}
            </div>
          )}
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => nav("/console/help")}
            title={t("kbd.title")}
            aria-label={t("kbd.title")}
            className="flex h-9 w-9 items-center justify-center border border-line text-mut transition-colors hover:border-line2 hover:text-fg"
          >
            <Keyboard size={14} aria-hidden />
          </button>
          <Btn onClick={() => setSelectMode(!selectMode)} variant={selectMode ? "solid" : "ghost"}>
            <ListChecks size={12} aria-hidden />
            {t("common.all")}
          </Btn>
          <Btn variant="solid" onClick={() => setShowUpload(!showUpload)}>
            <Upload size={12} aria-hidden />
            {t("con.upload")}
          </Btn>
        </div>
      </div>

      {showUpload && (
        <div className="mb-6">
          <UploadZone onFiles={(fs) => runUpload(fs)} />
        </div>
      )}

      <div className="mb-5 flex flex-wrap items-center gap-2">
        <div className="flex border border-line">
          {(["", "image", "audio"] as KindFilter[]).map((k) => (
            <button
              key={k || "all"}
              onClick={() => {
                setKind(k);
                setOffset(0);
              }}
              className={`micro px-3 py-2 ${kind === k ? "bg-fg text-ink" : "text-mut hover:text-fg"}`}
            >
              {k === "" ? t("common.all") : k === "image" ? t("common.image") : t("common.audio")}
            </button>
          ))}
        </div>
        <div className="flex border border-line">
          {(["", "public", "private"] as VisFilter[]).map((v) => (
            <button
              key={v || "all"}
              onClick={() => {
                setVis(v);
                setOffset(0);
              }}
              className={`micro px-3 py-2 ${vis === v ? "bg-fg text-ink" : "text-mut hover:text-fg"}`}
            >
              {v === "" ? t("con.vis.all") : v === "public" ? t("con.vis.public") : t("con.vis.private")}
            </button>
          ))}
        </div>
        <select
          value={sort}
          onChange={(e) => {
            setSort(e.target.value as SortKey);
            setOffset(0);
          }}
          className="micro border border-line bg-panel px-2 py-2 text-mut focus:border-line2 focus:outline-none"
        >
          <option value="newest">{t("con.sort.newest")}</option>
          <option value="oldest">{t("con.sort.oldest")}</option>
          <option value="views">{t("con.sort.views")}</option>
          <option value="largest">{t("con.sort.largest")}</option>
          <option value="smallest">{t("con.sort.smallest")}</option>
          <option value="name">{t("con.sort.name")}</option>
        </select>
        <input
          ref={searchRef}
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setOffset(0);
          }}
          placeholder={t("common.search")}
          className="micro min-w-[140px] flex-1 border border-line bg-panel px-3 py-2 text-mut placeholder:text-dim focus:border-line2 focus:outline-none"
        />
      </div>

      {selectMode && (
        <div className="mb-5 flex flex-wrap items-center gap-3 border border-line bg-panel px-4 py-3">
          <Micro>
            {t("con.selected", { n: selected.size })}
          </Micro>
          <button onClick={toggleAll} className="micro flex items-center gap-1 text-mut hover:text-fg">
            {allOnPage ? <X size={12} aria-hidden /> : <ListChecks size={12} aria-hidden />}
            {t("common.all")}
          </button>
          <span className="flex-1" />
          {selected.size > 0 && (
            <>
              <Btn
                onClick={() => doBulk(() => bulkVisibility(Array.from(selected), "public"))}
                disabled={bulkBusy}
              >
                <Globe size={12} aria-hidden /> {t("con.makePublic")}
              </Btn>
              <Btn
                onClick={() => doBulk(() => bulkVisibility(Array.from(selected), "private"))}
                disabled={bulkBusy}
              >
                <LockOpen size={12} aria-hidden /> {t("con.makePrivate")}
              </Btn>
              <Btn variant="danger" onClick={() => doBulk(() => bulkDelete(Array.from(selected)))} disabled={bulkBusy}>
                <Trash2 size={12} aria-hidden /> {t("con.deleteSelected")}
              </Btn>
            </>
          )}
        </div>
      )}

      {isLoading && <Spinner label={t("con.loading")} />}
      {error && !isLoading && (
        <ErrorBox text={t("con.loadError")} onRetry={() => mutate()} />
      )}
      {!isLoading && !error && items.length === 0 && uploads.length === 0 && (
        <Empty
          title={t("con.empty")}
          desc={t("con.empty.d")}
          action={
            <Btn variant="solid" onClick={() => setShowUpload(true)}>
              <Plus size={12} aria-hidden /> {t("con.upload")}
            </Btn>
          }
        />
      )}
      {!isLoading && (
        <MediaGrid
          items={items}
          uploads={uploads}
          selectable={selectMode}
          selectedIds={selected}
          onToggle={toggle}
        />
      )}

      {(() => {
        const pageCount = Math.max(1, Math.ceil(total / PAGE));
        return (
          <div className="mt-8">
            <Pager
              page={Math.floor(offset / PAGE) + 1}
              pageCount={pageCount}
              onChange={(p) => {
                setOffset((p - 1) * PAGE);
                window.scrollTo({ top: 0, behavior: "smooth" });
              }}
            />
            <div className="micro mt-2 text-center text-dim">
              {total} {t("common.all").toLowerCase()} · {t("con.page")} {Math.floor(offset / PAGE) + 1}/{pageCount}
            </div>
          </div>
        );
      })()}
    </div>
  );
}
