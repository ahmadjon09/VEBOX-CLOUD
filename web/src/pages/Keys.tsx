
import { useState } from "react";
import { AlertTriangle, KeyRound, Plus, Trash2 } from "lucide-react";
import { Link, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { useI18n } from "../i18n/context";
import { Seo } from "../components/Seo";
import { Btn, ConfirmBody, Copy, Empty, ErrorBox, Field, Input, Micro, Modal, Spinner, useToast } from "../components/ui";
import { createKey, deleteAllKeys, deleteKey, useKeys, type KeyItem } from "../services/media";
import { dateFmt, keyMask, timeAgo } from "../utils/format";

function KeysList() {
  const { t, lang } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const { data, error, isLoading, mutate } = useKeys();
  const [delAllBusy, setDelAllBusy] = useState(false);

  const keys: KeyItem[] = data?.data || [];

  const delAll = async () => {
    setDelAllBusy(true);
    try {
      await deleteAllKeys();
      toast(t("toast.deleted"));
      await mutate();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setDelAllBusy(false);
    }
  };

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <Seo title={`${t("keys.title")} — VEBOX`} path="/keys" />

      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl tracking-tight text-fg">{t("keys.title")}</h1>
          <p className="mt-2 max-w-md text-xs leading-relaxed text-mut">{t("keys.desc")}</p>
        </div>
        <div className="flex gap-2">
          {keys.length > 0 && (
            <Btn variant="danger" onClick={delAll} disabled={delAllBusy}>
              <Trash2 size={12} aria-hidden /> {t("keys.delAll")}
            </Btn>
          )}
          <Link to="new">
            <Btn variant="solid">
              <Plus size={12} aria-hidden /> {t("keys.new")}
            </Btn>
          </Link>
        </div>
      </div>

      {isLoading && <Spinner label={t("common.loading")} />}
      {error && !isLoading && <ErrorBox text={t("err.generic")} onRetry={() => mutate()} />}
      {!isLoading && !error && keys.length === 0 && (
        <Empty
          title={t("keys.empty")}
          action={
            <Link to="new">
              <Btn variant="solid">+ {t("keys.new")}</Btn>
            </Link>
          }
        />
      )}

      {!isLoading && keys.length > 0 && (
        <div className="space-y-px border border-line bg-line">
          {keys.map((k) => (
            <div key={k.id} className="grid gap-3 bg-ink px-4 py-4 sm:grid-cols-[1fr_auto_auto_auto] sm:items-center">
              <div className="flex min-w-0 items-center gap-2.5">
                <KeyRound size={14} className="shrink-0 text-teal" aria-hidden />
                <div className="min-w-0">
                  <div className="truncate text-sm text-fg">{k.name || keyMask(k.prefix)}</div>
                  <div className="micro mt-1 text-dim">{keyMask(k.prefix)}</div>
                </div>
              </div>
              <div className="micro text-mut">
                {t("keys.created")}: {dateFmt(k.created_at, lang)}
              </div>
              <div className="micro text-mut">
                {k.last_used_at ? timeAgo(k.last_used_at, lang) : t("keys.never")}
              </div>
              <Link to={`${k.id}/delete`}>
                <Btn variant="danger" className="px-3 py-2">
                  <Trash2 size={12} aria-hidden /> {t("common.delete")}
                </Btn>
              </Link>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function NewKeyModal() {
  const { t } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [plain, setPlain] = useState("");

  const create = async () => {
    setBusy(true);
    try {
      const r = await createKey(name || "");
      setPlain(r.plain);
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={t("keys.new")} onClose={() => nav("/keys")} wide>
      {!plain ? (
        <div className="space-y-4">
          <Field label={t("keys.name")}>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="ci, script, partner…"
              onKeyDown={(e) => e.key === "Enter" && create()}
            />
          </Field>
          <Btn variant="solid" onClick={create} loading={busy} className="w-full">
            {t("keys.create")}
          </Btn>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="micro flex items-start gap-2 text-warn">
            <AlertTriangle size={13} className="mt-0.5 shrink-0" aria-hidden /> {t("keys.plainWarn")}
          </div>
          <div className="flex items-center gap-2 border border-line bg-ink px-3 py-3">
            <code className="min-w-0 flex-1 break-all text-xs text-fg">{plain}</code>
            <Copy text={plain} label={t("common.copy")} />
          </div>
          <Btn onClick={() => nav("/keys")} className="w-full">
            {t("common.close")}
          </Btn>
        </div>
      )}
    </Modal>
  );
}

function DeleteKeyModal() {
  const { id } = useParams<{ id: string }>();
  const { t } = useI18n();
  const nav = useNavigate();
  const toast = useToast();
  const { mutate } = useKeys();
  const [busy, setBusy] = useState(false);

  const confirm = async () => {
    if (!id) return;
    setBusy(true);
    try {
      await deleteKey(id);
      toast(t("toast.deleted"));
      nav("/keys");
      setTimeout(() => mutate(), 500);
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
      setBusy(false);
    }
  };

  return (
    <Modal title={t("keys.del.title")} onClose={() => nav("/keys")}>
      <ConfirmBody
        desc={t("keys.del.desc")}
        confirmLabel={t("common.delete")}
        busy={busy}
        onConfirm={confirm}
        onCancel={() => nav("/keys")}
      />
    </Modal>
  );
}

export function KeysPage() {
  return (
    <>
      <KeysList />
      <Routes>
        <Route path="new" element={<NewKeyModal />} />
        <Route path=":id/delete" element={<DeleteKeyModal />} />
      </Routes>
    </>
  );
}
