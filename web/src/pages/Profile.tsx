
import { useEffect, useRef, useState } from "react";
import { Camera, CircleX, KeyRound, Save } from "lucide-react";
import { Link, Route, Routes, useNavigate } from "react-router-dom";
import { useI18n } from "../i18n/context";
import { useAuth } from "../providers/AuthProvider";
import { Seo } from "../components/Seo";
import { Btn, ErrorBox, Field, Input, Micro, Modal, useToast } from "../components/ui";
import { getSettings, updateSettings, uploadAvatar, useAccount } from "../services/media";
import { api } from "../services/api";
import { assetUrl } from "../services/api";
import { dateFmt } from "../utils/format";

async function patchProfile(name: string): Promise<void> {
  await api.patch("/v1/profile", { name });
}

async function changePassword(current: string, neu: string): Promise<void> {
  await api.post("/v1/auth/change-password", { current, new: neu });
}

function ProfileBody() {
  const { t, lang } = useI18n();
  const toast = useToast();
  const { session, refresh } = useAuth();
  const u = session?.user;
  const [name, setName] = useState(u?.name || "");
  const [nameDirty, setNameDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [avatarBusy, setAvatarBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  if (!u) return <ErrorBox text={t("err.noSession")} />;

  const save = async () => {
    setBusy(true);
    try {
      await patchProfile(name);
      toast(t("pf.saved"));
      setNameDirty(false);
      await refresh();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setBusy(false);
    }
  };

  const onAvatar = async (f: File | null) => {
    if (!f) return;
    setAvatarBusy(true);
    try {
      await uploadAvatar(f);
      toast(t("toast.saved"));
      await refresh();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setAvatarBusy(false);
    }
  };

  return (
    <div className="mx-auto max-w-2xl px-4 py-8">
      <Seo title={`${t("pf.title")} — VEBOX`} path="/profile" />
      <h1 className="mb-8 text-2xl tracking-tight text-fg">{t("pf.title")}</h1>

      <div className="mb-6 flex items-center gap-5 border border-line bg-panel p-5">
        {u.avatar_url ? (
          <img src={assetUrl(u.avatar_url)} alt="" className="h-16 w-16 object-cover" />
        ) : (
          <div className="flex h-16 w-16 items-center justify-center bg-panel2 text-xl text-mut">
            {(u.name || u.email).slice(0, 2).toUpperCase()}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <Micro>{t("pf.avatar")}</Micro>
          <p className="mt-1.5 text-[11px] leading-relaxed text-mut">{t("pf.avatar.hint")}</p>
        </div>
        <input
          ref={fileRef}
          type="file"
          accept="image/jpeg,image/png,image/webp,image/bmp,image/tiff,image/x-icon,image/svg+xml,image/avif,image/heic"
          className="hidden"
          onChange={(e) => {
            onAvatar(e.target.files?.[0] || null);
            e.target.value = "";
          }}
        />
        <Btn onClick={() => fileRef.current?.click()} loading={avatarBusy}>
          <Camera size={12} aria-hidden /> {t("pf.avatar.change")}
        </Btn>
      </div>

      <div className="mb-6 space-y-3 border border-line bg-panel p-5">
        <Field label={t("pf.name")}>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setNameDirty(true);
            }}
            placeholder={u.email}
          />
        </Field>
        {nameDirty && (
          <Btn variant="solid" onClick={save} loading={busy} className="w-full sm:w-auto">
            <Save size={12} aria-hidden /> {t("common.save")}
          </Btn>
        )}
      </div>

      <h2 className="mb-3 mt-8 border-b border-line pb-2 text-xs uppercase tracking-widest text-mut">
        {t("pf.settings")}
      </h2>
      <HDSection />

      <div className="mb-6 grid gap-px border border-line bg-line sm:grid-cols-3">
        <div className="bg-ink px-4 py-3">
          <Micro>{t("pf.email")}</Micro>
          <div className="mt-1.5 break-all text-xs text-fg">{u.email}</div>
        </div>
        <div className="bg-ink px-4 py-3">
          <Micro>{t("pf.provider")}</Micro>
          <div className="mt-1.5 text-xs uppercase text-fg">{u.provider}</div>
        </div>
        <div className="bg-ink px-4 py-3">
          <Micro>{t("pf.member")}</Micro>
          <div className="mt-1.5 text-xs text-fg">{dateFmt(u.created_at, lang)}</div>
        </div>
      </div>

      <div className="flex items-center justify-between border border-line bg-panel px-5 py-4">
        <div>
          <Micro>{t("pf.pw.title")}</Micro>
          {u.provider !== "email" && (
            <div className="mt-1.5 text-[11px] text-mut">{t("pf.pw.githubNote")}</div>
          )}
        </div>
        {u.provider === "email" && (
          <Link to="password">
            <Btn>
              <KeyRound size={12} aria-hidden /> {t("pf.pw.change")}
            </Btn>
          </Link>
        )}
      </div>
    </div>
  );
}

function HDSection() {
  const { t } = useI18n();
  const toast = useToast();
  const { mutate: mutateAccount } = useAccount();
  const [hd, setHd] = useState<boolean | null>(null);
  const [maxWidth, setMaxWidth] = useState(0);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    getSettings()
      .then((s) => {
        if (!alive) return;
        setHd(!!s.hd_processing);
        setMaxWidth(s.max_hd_width || 0);
      })
      .catch(() => {
        if (alive) setHd(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  const toggle = async (next: boolean) => {
    setBusy(true);
    setHd(next);
    try {
      const s = await updateSettings({ hd_processing: next });
      setHd(!!s.hd_processing);
      setMaxWidth(s.max_hd_width || 0);
      mutateAccount();
      toast(t("pf.hd.saved"));
    } catch (e: any) {
      setHd(!next);
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mb-6 border border-line bg-panel px-5 py-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <Micro>{t("pf.hd.title")}</Micro>
          <p className="mt-1.5 max-w-lg text-[11px] leading-relaxed text-mut">
            {t("pf.hd.hint", { width: maxWidth > 0 ? String(maxWidth) : "" })}
          </p>
          {hd === false && (
            <p className="micro mt-2 flex items-center gap-1.5 text-warn">
              <CircleX size={12} aria-hidden /> {t("pf.hd.off")}
            </p>
          )}
        </div>
        <button
          role="switch"
          aria-checked={hd === true}
          aria-label={t("pf.hd.title")}
          disabled={busy || hd === null}
          onClick={() => toggle(!hd)}
          className={`relative h-6 w-11 shrink-0 border transition-colors disabled:opacity-40 ${
            hd ? "border-fg bg-fg" : "border-line2 bg-panel2"
          }`}
        >
          <span
            className={`absolute left-[3px] top-[3px] h-[18px] w-[18px] transition-transform ${
              hd ? "translate-x-[20px] bg-ink" : "bg-mut"
            }`}
          />
        </button>
      </div>
    </div>
  );
}

function PasswordModal() {
  const { t } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const [cur, setCur] = useState("");
  const [neu, setNeu] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submit = async () => {
    setErr("");
    if (neu.length < 8) {
      setErr(t("auth.pwHint"));
      return;
    }
    setBusy(true);
    try {
      await changePassword(cur, neu);
      toast(t("pf.pw.done"));
      nav("/profile");
    } catch (e: any) {
      const code = e?.response?.data?.error?.code;
      setErr(code === "wrong_password" ? t("auth.err.badCreds") : e?.response?.data?.error?.message || t("err.generic"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={t("pf.pw.title")} onClose={() => nav("/profile")}>
      <div className="space-y-4">
        <Field label={t("pf.pw.current")}>
          <Input type="password" value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" />
        </Field>
        <Field label={`${t("pf.pw.new")} — ${t("auth.pwHint")}`}>
          <Input
            type="password"
            value={neu}
            onChange={(e) => setNeu(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            autoComplete="new-password"
          />
        </Field>
        {err && (
          <div className="micro flex items-center gap-1.5 text-err">
            <CircleX size={12} aria-hidden /> {err}
          </div>
        )}
        <Btn variant="solid" onClick={submit} loading={busy} disabled={!cur || neu.length < 8} className="w-full">
          {t("pf.pw.change")}
        </Btn>
      </div>
    </Modal>
  );
}

export function ProfilePage() {
  return (
    <>
      <ProfileBody />
      <Routes>
        <Route path="password" element={<PasswordModal />} />
      </Routes>
    </>
  );
}
