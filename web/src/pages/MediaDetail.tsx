
import { useEffect, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Download,
  Globe,
  Lock,
  Share2,
  Trash2,
} from "lucide-react";
import { Link, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { useI18n } from "../i18n/context";
import { useHotkeys } from "../hooks/useHotkeys";
import { Seo } from "../components/Seo";
import { Btn, ConfirmBody, Copy, ErrorBox, Micro, Modal, Spinner, useToast } from "../components/ui";
import {
  createSignedLink,
  deleteMedia,
  deliveryUrl,
  downloadOriginal,
  setVisibility,
  useMedia,
  useMediaList,
  type SignedLink,
} from "../services/media";
import { assetUrl } from "../services/api";
import { AudioPlayer } from "../components/AudioPlayer";
import { PaletteBars, cleanColors } from "../components/Palette";
import { bytes, dateFmt, durFmt } from "../utils/format";

function DetailContent({ id }: { id: string }) {
  const { t, lang } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const { data: item, error, isLoading, mutate } = useMedia(id);
  const [visBusy, setVisBusy] = useState(false);
  const [dlBusy, setDlBusy] = useState(false);

  const { data: list } = useMediaList();
  const siblings: { id: string }[] = list?.data || [];

  useHotkeys({
    Escape: () => nav("/console"),
    ArrowLeft: () => {
      const i = siblings.findIndex((s) => s.id === id);
      if (i > 0) nav("/media/" + siblings[i - 1].id);
    },
    ArrowRight: () => {
      const i = siblings.findIndex((s) => s.id === id);
      if (i >= 0 && i < siblings.length - 1) nav("/media/" + siblings[i + 1].id);
    },
  });

  if (isLoading) return <Spinner label={t("common.loading")} />;
  if (error || !item) return <ErrorBox text={t("md.notFound")} />;

  const isAudio = item.kind === "audio";
  const palette = cleanColors(item.colors);
  const dims = item.width && item.height ? `${item.width} × ${item.height}` : "—";

  const download = async () => {
    setDlBusy(true);
    try {
      await downloadOriginal({ id: item.id, filename: item.filename, urls: item.urls });
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setDlBusy(false);
    }
  };

  const sibIdx = siblings.findIndex((s) => s.id === id);
  const goPrev = sibIdx > 0 ? () => nav("/media/" + siblings[sibIdx - 1].id) : null;
  const goNext = sibIdx >= 0 && sibIdx < siblings.length - 1 ? () => nav("/media/" + siblings[sibIdx + 1].id) : null;

  const flipVis = async () => {
    setVisBusy(true);
    try {
      await setVisibility(item.id, item.visibility === "public" ? "private" : "public");
      toast(t("toast.saved"));
      await mutate();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setVisBusy(false);
    }
  };

  return (
    <div className="mx-auto max-w-5xl px-4 py-8">
      <Seo title={`${item.filename} — VEBOX`} desc={`${item.mime_type} · ${bytes(item.size_bytes)}`} path={`/media/${item.id}`} />

      <div className="mb-5 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <Link to="/console" className="micro flex shrink-0 items-center gap-1.5 text-mut hover:text-fg">
            <ArrowLeft size={12} aria-hidden /> {t("nav.files")}
          </Link>
          <span className="truncate text-sm text-fg" title={item.filename}>
            {item.filename}
          </span>
        </div>
        <div className="flex gap-2">
          <button
            onClick={download}
            disabled={dlBusy}
            className="micro flex items-center gap-1.5 border border-line px-3 py-2 text-fg transition-colors hover:border-line2 disabled:opacity-50"
          >
            <Download size={12} aria-hidden /> {t("md.download")}
          </button>
          <Link
            to="share"
            className="micro flex items-center gap-1.5 border border-line px-3 py-2 text-fg hover:border-line2"
          >
            <Share2 size={12} aria-hidden /> {t("md.share")}
          </Link>
          <Link
            to="delete"
            className="micro flex items-center gap-1.5 border border-err/40 px-3 py-2 text-err hover:bg-err/10"
          >
            <Trash2 size={12} aria-hidden /> {t("common.delete")}
          </Link>
        </div>
      </div>

      <div className="relative border border-line bg-panel">
        {goPrev && (
          <button
            onClick={goPrev}
            aria-label="Oldingi"
            className="absolute left-2 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center border border-line bg-ink/80 text-mut transition-colors hover:border-line2 hover:text-fg"
          >
            <ArrowLeft size={16} aria-hidden />
          </button>
        )}
        {goNext && (
          <button
            onClick={goNext}
            aria-label="Keyingi"
            className="absolute right-2 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center border border-line bg-ink/80 text-mut transition-colors hover:border-line2 hover:text-fg"
          >
            <ArrowRight size={16} aria-hidden />
          </button>
        )}
        {isAudio ? (
          <div className="flex flex-col items-center gap-6 p-10">
            <AudioPlayer src={deliveryUrl(item)} mime={item.mime_type} />
            <Micro>
              {item.mime_type.replace("audio/", "").toUpperCase()} · {bytes(item.size_bytes)}
              {item.duration_fmt ? ` · ${durFmt(item.duration)}` : ""}
            </Micro>
          </div>
        ) : (
          <div
            className="flex items-center justify-center p-2"
            style={{ backgroundColor: item.bg && /^#[0-9a-fA-F]{6}$/.test(item.bg) ? item.bg : "#000" }}
          >
            <img
              src={deliveryUrl(item)}
              alt={item.filename}
              className="max-h-[70vh] w-auto object-contain"
            />
          </div>
        )}
      </div>

      <div className="mt-4 grid gap-px border border-line bg-line sm:grid-cols-2 lg:grid-cols-4">
        <div className="bg-ink px-4 py-3">
          <Micro>{t("md.type")}</Micro>
          <div className="mt-1.5 text-xs text-fg">{item.mime_type || item.kind}</div>
        </div>
        <div className="bg-ink px-4 py-3">
          <Micro>{t("md.dimensions")}</Micro>
          <div className="mt-1.5 text-xs text-fg">{isAudio ? "—" : dims}</div>
        </div>
        <div className="bg-ink px-4 py-3">
          <Micro>{t("common.size")}</Micro>
          <div className="mt-1.5 text-xs text-fg">{bytes(item.size_bytes)}</div>
        </div>
        <div className="bg-ink px-4 py-3">
          <Micro>{t("md.created")}</Micro>
          <div className="mt-1.5 text-xs text-fg">{dateFmt(item.created_at, lang)}</div>
        </div>
        <div className="bg-ink px-4 py-3">
          <Micro>{t("common.views")}</Micro>
          <div className="mt-1.5 text-xs text-fg">{item.views}</div>
        </div>
      </div>

      <div className="mt-4 grid gap-px border border-line bg-line sm:grid-cols-2">
        <div className="flex items-center justify-between bg-ink px-4 py-3">
          <div>
            <Micro>{t("md.checksum")}</Micro>
            <div className="mt-1.5 max-w-[220px] truncate text-[10px] text-mut">{item.checksum}</div>
          </div>
          <Copy text={item.checksum} label={t("common.copy")} />
        </div>
        <div className="flex items-center justify-between bg-ink px-4 py-3">
          <div>
            <Micro>{item.visibility === "public" ? t("common.public") : t("common.private")}</Micro>
            <div className="mt-1.5 text-[10px] text-mut">{item.id}</div>
          </div>
          <Btn onClick={flipVis} disabled={visBusy} className="px-3 py-2">
            {item.visibility === "public" ? (
              <>
                <Lock size={12} aria-hidden /> {t("md.vis.toPrivate")}
              </>
            ) : (
              <>
                <Globe size={12} aria-hidden /> {t("md.vis.toPublic")}
              </>
            )}
          </Btn>
        </div>
      </div>

      {palette.length > 0 && <PaletteBars colors={item.colors} />}

      <div className="mt-4 grid gap-px border border-line bg-line sm:grid-cols-2">
        <div className="flex items-center justify-between bg-ink px-4 py-3">
          <div className="min-w-0">
            <Micro>{t("md.link.cdn")}</Micro>
            <div className="mt-1.5 max-w-[260px] truncate text-[10px] text-mut">{assetUrl(item.urls.cdn)}</div>
          </div>
          <Copy text={assetUrl(item.urls.cdn)} label={t("md.copyLink")} />
        </div>
        <div className="flex items-center justify-between bg-ink px-4 py-3">
          <div className="min-w-0">
            <Micro>{t("md.link.preview")}</Micro>
            <div className="mt-1.5 max-w-[260px] truncate text-[10px] text-mut">{assetUrl(item.urls.preview)}</div>
          </div>
          <Copy text={assetUrl(item.urls.preview)} label={t("md.copyLink")} />
        </div>
      </div>

      {item.visibility === "private" && (
        <div className="micro mt-3 text-dim">{t("md.private.note")}</div>
      )}
    </div>
  );
}

function ShareModal({ id }: { id: string }) {
  const { t } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const { data: item } = useMedia(id);
  const [ttl, setTtl] = useState(24 * 3600);
  const [busy, setBusy] = useState(false);
  const [link, setLink] = useState<SignedLink | null>(null);

  const ttlOpts = [
    { v: 3600, k: "md.ttl.1h" },
    { v: 24 * 3600, k: "md.ttl.24h" },
    { v: 7 * 86400, k: "md.ttl.7d" },
    { v: 30 * 86400, k: "md.ttl.30d" },
  ];

  const create = async () => {
    setBusy(true);
    try {
      setLink(await createSignedLink(id, ttl));
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setBusy(false);
    }
  };

  if (item?.visibility === "public") {
    return (
      <Modal title={t("md.share.title")} onClose={() => nav("/media/" + id)}>
        <div className="space-y-4">
          <Micro className="flex items-center gap-1.5 text-ok">
            <Globe size={12} aria-hidden /> {t("common.public")}
          </Micro>
          <div className="flex items-center justify-between border border-line bg-ink px-3 py-2.5">
            <span className="min-w-0 truncate text-[11px] text-mut">{assetUrl(item.urls.cdn)}</span>
            <Copy text={assetUrl(item.urls.cdn)} label={t("md.copyLink")} />
          </div>
          <Btn onClick={() => nav("/media/" + id)} className="w-full">
            {t("common.close")}
          </Btn>
        </div>
      </Modal>
    );
  }

  return (
    <Modal title={t("md.share.title")} onClose={() => nav("/media/" + id)}>
      <div className="space-y-4">
        <p className="text-xs leading-relaxed text-mut">{t("md.share.desc")}</p>

        {!link ? (
          <>
            <div className="flex flex-wrap gap-2">
              {ttlOpts.map((o) => (
                <button
                  key={o.v}
                  onClick={() => setTtl(o.v)}
                  className={`micro border px-3 py-2 ${
                    ttl === o.v ? "border-fg bg-fg text-ink" : "border-line text-mut hover:text-fg"
                  }`}
                >
                  {t(o.k)}
                </button>
              ))}
            </div>
            <Btn variant="solid" onClick={create} loading={busy} className="w-full">
              {t("md.share.create")}
            </Btn>
          </>
        ) : (
          <div className="space-y-3">
            <Micro className="flex items-center gap-1.5 text-ok">
              <Check size={12} aria-hidden /> {t("md.share.created")}
            </Micro>
            <div className="flex items-center gap-2 border border-line bg-ink px-3 py-2.5">
              <span className="min-w-0 flex-1 truncate text-[11px] text-mut">{link.url}</span>
              <Copy text={link.url} label={t("common.copy")} />
            </div>
            <div className="micro text-dim">{link.expires_at}</div>
          </div>
        )}
      </div>
    </Modal>
  );
}

function DeleteModal({ id }: { id: string }) {
  const { t } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  const confirm = async () => {
    setBusy(true);
    try {
      await deleteMedia(id);
      toast(t("toast.deleted"));
      nav("/console");
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
      setBusy(false);
    }
  };

  return (
    <Modal title={t("md.delete.title")} onClose={() => nav("/media/" + id)}>
      <ConfirmBody
        desc={t("md.delete.desc")}
        confirmLabel={t("common.delete")}
        busy={busy}
        onConfirm={confirm}
        onCancel={() => nav("/media/" + id)}
      />
    </Modal>
  );
}

export function MediaDetail() {
  const { id } = useParams<{ id: string }>();
  return (
    <>
      <DetailContent id={id || ""} />
      <Routes>
        <Route path="share" element={<ShareModal id={id || ""} />} />
        <Route path="delete" element={<DeleteModal id={id || ""} />} />
      </Routes>
    </>
  );
}
