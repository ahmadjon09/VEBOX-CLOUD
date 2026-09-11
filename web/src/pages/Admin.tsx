import { useState } from "react";
import { Ban, Eraser, RefreshCw, ShieldCheck } from "lucide-react";
import { useI18n } from "../i18n/context";
import { useIsAdmin } from "../providers/AuthProvider";
import { Seo } from "../components/Seo";
import { Btn, Empty, ErrorBox, Micro, Spinner, Stat, useToast } from "../components/ui";
import {
  adminInvalidateCache,
  adminSetAdmin,
  adminSetBan,
  adminSetTier,
  useAdminSystem,
  useAdminUsers,
  type AdminUserRow,
  type DbStats,
} from "../services/media";
import { bytes } from "../utils/format";

const TIERS = ["free", "business"];

function UserRow({ row, onChanged }: { row: AdminUserRow; onChanged: () => void }) {
  const { t } = useI18n();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const u = row.user;
  const raw = row.effective_policy?.tier || row.subscription?.tier || "free";
  const tier = raw === "business" ? "business" : "free";
  const used = row.usage?.storage_bytes || 0;
  const quota = (row.effective_policy?.storage_mb || 0) * 1024 * 1024;

  const act = async (fn: () => Promise<void>, okMsg: string) => {
    setBusy(true);
    try {
      await fn();
      toast(okMsg);
      onChanged();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={`grid gap-3 bg-ink px-4 py-4 sm:grid-cols-[1fr_auto_auto_auto_auto] sm:items-center ${u.banned ? "opacity-60" : ""}`}>
      <div className="min-w-0">
        <div className="truncate text-sm text-fg">
          {u.name || u.email} {u.is_admin && <span className="micro text-warn">· ADMIN</span>} {u.banned && <span className="micro text-err">· {t("ad.banned")}</span>}
        </div>
        <div className="micro mt-1 truncate text-dim">{u.email}</div>
        <div className="micro mt-1 text-dim">
          {quota > 0 ? `${bytes(used)} / ${bytes(quota)}` : bytes(used)} · {row.usage?.file_count ?? 0}
        </div>
      </div>

      <select
        value={tier}
        disabled={busy}
        onChange={(e) => act(() => adminSetTier(u.id, e.target.value), t("toast.saved"))}
        className="micro border border-line bg-panel px-2 py-1.5 text-mut focus:border-line2 focus:outline-none"
        aria-label={t("ad.tier")}
      >
        {TIERS.map((x) => (
          <option key={x} value={x}>{x.toUpperCase()}</option>
        ))}
      </select>

      <Btn
        onClick={() => act(() => adminSetBan(u.id, !u.banned), t("toast.saved"))}
        disabled={busy}
        variant={u.banned ? "solid" : "ghost"}
        className={`px-3 py-1.5 ${u.banned ? "" : "text-warn"}`}
      >
        {u.banned ? <ShieldCheck size={12} aria-hidden /> : <Ban size={12} aria-hidden />}
        {u.banned ? t("ad.unban") : t("ad.ban")}
      </Btn>

      <Btn
        onClick={() => act(() => adminSetAdmin(u.id, !u.is_admin), t("toast.saved"))}
        disabled={busy}
        variant={u.is_admin ? "solid" : "ghost"}
        className={`px-3 py-1.5 ${u.is_admin ? "" : "text-warn"}`}
      >
        {u.is_admin ? t("ad.revokeAdmin") : t("ad.makeAdmin")}
      </Btn>

      <span className="hidden text-right sm:block">
        <span className={`micro ${u.banned ? "text-err" : "text-mut"}`}>{u.provider.toUpperCase()}</span>
      </span>
    </div>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="border border-line bg-panel p-4">
      <Micro className="mb-3">{title}</Micro>
      {children}
    </div>
  );
}

function KV({ k, v, mono = true }: { k: string; v: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3 text-xs">
      <span className="micro shrink-0 text-dim">{k}</span>
      <span className={`min-w-0 truncate text-right text-fg ${mono ? "text-[11px]" : ""}`}>{v}</span>
    </div>
  );
}

function Bar({ pct, warn }: { pct: number; warn?: boolean }) {
  const w = Math.max(0, Math.min(100, pct || 0));
  return (
    <div className="mt-3 h-1 w-full bg-panel2">
      <div className={`h-full ${warn ? "bg-warn" : "bg-ok"}`} style={{ width: `${w}%` }} />
    </div>
  );
}

function DbPanel({ label, db }: { label: string; db?: DbStats }) {
  const { t } = useI18n();
  if (!db || db.connected === false) {
    return (
      <Panel title={label}>
        <div className="text-xs text-err">{t("ad.notConnected")}{db?.error ? ` — ${db.error}` : ""}</div>
      </Panel>
    );
  }
  const size = db.size_bytes || db.data_bytes || 0;
  return (
    <Panel title={label}>
      <div className="space-y-1.5">
        <KV k={t("ad.database")} v={db.engine || "—"} />
        <KV k={t("ad.db")} v={db.database || "—"} />
        {db.server_version && <KV k={t("ad.version")} v={db.server_version} />}
        <KV k={t("ad.dbSize")} v={size ? bytes(size) : "—"} />
        {db.free_bytes !== undefined && <KV k={t("ad.dbFree")} v={bytes(db.free_bytes)} />}
        {db.used_percent !== undefined && <KV k="%" v={`${db.used_percent}%`} />}
        {db.tables && db.tables.length > 0 && (
          <>
            <div className="micro pt-2 text-dim">{t("ad.dbTables")}</div>
            {db.tables.map((tb) => (
              <KV key={tb.name} k={tb.name} v={`${tb.rows} ${t("ad.dbRows")} · ${bytes(tb.bytes || 0)}`} />
            ))}
          </>
        )}
        {db.databases && db.databases.length > 0 && (
          <>
            <div className="micro pt-2 text-dim">{t("ad.db")}</div>
            {db.databases.map((d) => (
              <KV key={d.name} k={d.name} v={bytes(d.size_bytes || 0)} />
            ))}
          </>
        )}
        {db.pool && (
          <KV k={t("ad.dbPool")} v={`${db.pool.in_use}/${db.pool.open}${db.pool.max_open ? ` (${db.pool.max_open})` : ""}`} />
        )}
        {db.images !== undefined && <KV k={t("nav.files")} v={db.images} />}
      </div>
    </Panel>
  );
}

export function Admin() {
  const { t } = useI18n();
  const isAdmin = useIsAdmin();
  const toast = useToast();
  const { data, error, isLoading, mutate } = useAdminUsers(0);
  const { data: sys, mutate: mutateSys } = useAdminSystem();
  const [tab, setTab] = useState<"users" | "system">("users");
  const [clearing, setClearing] = useState(false);

  if (!isAdmin) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-16">
        <Seo title={t("ad.title")} path="/admin" />
        <Empty title={t("ad.needAdmin")} />
      </div>
    );
  }

  const rows: AdminUserRow[] = data?.data || [];
  const mem = sys?.memory;
  const sysinfo = sys?.system;
  const redis = sys?.redis;
  const jobs = sys?.jobs;
  const nodes = sys?.storage_backend?.nodes || [];
  const summary = sys?.storage_backend?.summary;
  const dbs = sys?.databases || {};
  const cfg = sys?.config || {};

  const memPct = mem?.used_percent ?? 0;
  const memBusy = (mem?.state || "ok") !== "ok";
  const redisPct = redis?.budget_bytes ? ((redis.used_memory || redis.cached_bytes || 0) / redis.budget_bytes) * 100 : 0;
  const diskPct = sysinfo?.disk?.used_percent ?? 0;

  const clearCache = async () => {
    setClearing(true);
    try {
      await adminInvalidateCache("all");
      toast(t("ad.cacheCleared"));
      mutateSys();
    } catch (e: any) {
      toast(e?.response?.data?.error?.message || t("err.generic"), "err");
    } finally {
      setClearing(false);
    }
  };

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <Seo title={t("ad.title")} path="/admin" />

      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <h1 className="text-2xl tracking-tight text-fg">{t("ad.title")}</h1>
        <div className="flex border border-line">
          {(["users", "system"] as const).map((x) => (
            <button
              key={x}
              onClick={() => setTab(x)}
              className={`micro px-3 py-2 ${tab === x ? "bg-fg text-ink" : "text-mut hover:text-fg"}`}
            >
              {t(x === "users" ? "ad.users" : "ad.system")}
            </button>
          ))}
        </div>
      </div>

      {tab === "users" && (
        <>
          {isLoading && <Spinner label={t("common.loading")} />}
          {error && !isLoading && <ErrorBox text={t("err.generic")} onRetry={() => mutate()} />}
          {!isLoading && !error && rows.length === 0 && <Empty title={t("ad.users")} />}
          {!isLoading && rows.length > 0 && (
            <div className="space-y-px border border-line bg-line">
              {rows.map((r) => (
                <UserRow key={r.user.id} row={r} onChanged={() => mutate()} />
              ))}
            </div>
          )}
        </>
      )}

      {tab === "system" && (
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-px border border-line bg-line lg:grid-cols-4">
            <Stat k={t("ad.uptime")} v={sys?.uptime || "—"} />
            <Stat k={t("ad.storage")} v={sys?.storage_backend?.mode || "—"} />
            <Stat k={t("ad.memory")} v={mem ? `${Math.round(memPct)}%` : "—"} />
            <Stat k={t("ad.bots")} v={`${summary?.available ?? 0}/${summary?.nodes ?? nodes.length}`} />
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Panel title={t("ad.runtime")}>
              <div className="space-y-1.5">
                <KV k={t("ad.version")} v={sys?.version || "—"} />
                <KV k="Go" v={sysinfo?.go_version || "—"} />
                <KV k={t("ad.os")} v={sysinfo ? `${sysinfo.os}/${sysinfo.arch}` : "—"} />
                <KV k={t("ad.host")} v={sysinfo?.hostname || "—"} />
                <KV k={t("ad.cpu")} v={sysinfo?.num_cpu ?? "—"} />
                <KV k={t("ad.goroutines")} v={sysinfo?.num_goroutine ?? mem?.goroutines ?? "—"} />
                <KV k={t("ad.gc")} v={sysinfo ? `${sysinfo.gc_pause_ms} ms (${sysinfo.num_gc})` : "—"} />
                <KV k="PID" v={sysinfo?.pid ?? "—"} />
                <KV k={t("ad.disk")} v={sysinfo?.disk ? `${bytes(sysinfo.disk.used_bytes)} / ${bytes(sysinfo.disk.total_bytes)}` : "—"} />
                {sysinfo?.disk && <KV k={t("ad.diskFree")} v={bytes(sysinfo.disk.avail_bytes)} />}
              </div>
              {sysinfo?.disk && <Bar pct={diskPct} warn={diskPct > 85} />}
            </Panel>

            <Panel title={t("ad.memory")}>
              <div className="space-y-1.5">
                <KV k={t("ad.state")} v={memBusy ? <span className="text-warn">{t("ad.busy")}</span> : t("ad.normal")} />
                <KV k="used" v={mem?.used_bytes !== undefined ? bytes(mem.used_bytes) : "—"} />
                <KV k="limit" v={mem?.limit_bytes ? bytes(mem.limit_bytes) : "—"} />
                <KV k="heap" v={mem?.heap_alloc !== undefined ? bytes(mem.heap_alloc) : "—"} />
                <KV k="sys" v={mem?.sys !== undefined ? bytes(mem.sys) : "—"} />
                <KV k={t("ad.goroutines")} v={mem?.goroutines ?? "—"} />
                <KV k={t("ad.jobs")} v={mem?.heavy_slots ? `${mem.heavy_inuse}/${mem.heavy_slots}` : "—"} />
              </div>
              <Bar pct={memPct} warn={memBusy} />
            </Panel>

            <Panel title={t("ad.redis")}>
              {!redis?.enabled ? (
                <div className="text-xs text-dim">{t("ad.redisOff")}</div>
              ) : (
                <>
                  <div className="space-y-1.5">
                    <KV k={t("ad.host")} v={redis.addr || "—"} />
                    <KV k={t("ad.state")} v={redis.connected ? <span className="text-ok">UP</span> : <span className="text-err">DOWN</span>} />
                    <KV k={t("ad.redisHit")} v={redis.hit_rate !== undefined ? `${redis.hit_rate}%` : "—"} />
                    <KV k="hits / misses" v={`${redis.hits ?? 0} / ${redis.misses ?? 0}`} />
                    <KV k={t("ad.redisCached")} v={`${bytes(redis.used_memory || redis.cached_bytes || 0)} / ${bytes(redis.budget_bytes || 0)}`} />
                    <KV k={t("ad.redisBudget")} v={bytes(redis.budget_bytes || 0)} />
                    <KV k={t("ad.redisErrors")} v={`${redis.errors ?? 0}${redis.skipped ? ` · +${redis.skipped}` : ""}`} />
                    <KV k="TTL" v={`${redis.meta_ttl || "—"} / ${redis.variant_ttl || "—"}`} />
                    {redis.policy && <KV k="maxmemory" v={`${redis.policy} · ${bytes(redis.server_maxmemory || 0)}`} />}
                    {redis.last_error && <KV k="last error" v={<span className="text-err">{redis.last_error}</span>} />}
                  </div>
                  <Bar pct={redisPct} warn={redisPct > 90} />
                </>
              )}
            </Panel>

            <Panel title={t("ad.jobs")}>
              <div className="space-y-1.5">
                <KV k={t("ad.jobsQueued")} v={jobs?.pending ?? "—"} />
                <KV k={t("ad.jobsActive")} v={jobs?.queue_len !== undefined ? `${jobs.queue_len}/${jobs.queue_cap ?? 0}` : "—"} />
                <KV k={t("ad.jobsStaging")} v={jobs?.staged_bytes !== undefined ? `${bytes(jobs.staged_bytes)} (${jobs.staging ?? 0}/${jobs.staging_cap ?? 0})` : "—"} />
                <KV k="meta cache" v={jobs?.meta_cache ?? "—"} />
                <KV k="dedup" v={jobs?.dedup ?? "—"} />
              </div>
            </Panel>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <DbPanel label="PostgreSQL" db={dbs.postgres} />
            <DbPanel label="MongoDB" db={dbs.mongo} />
          </div>

          <Panel title={t("ad.bots")}>
            {nodes.length === 0 ? (
              <div className="text-xs text-dim">{sys?.storage_backend?.mode === "local-ephemeral" ? "local" : "—"}</div>
            ) : (
              <>
                <div className="micro mb-2 flex flex-wrap gap-3 text-dim">
                  <span className="text-ok">{t("ad.botsUp")}: {summary?.available ?? 0}</span>
                  {Boolean(summary?.cooling) && <span className="text-warn">{t("ad.botsCooling")}: {summary?.cooling}</span>}
                  <span>ok: {summary?.ok ?? 0}</span>
                  <span>err: {summary?.failed ?? 0}</span>
                  <span>in-flight: {summary?.inflight ?? 0}</span>
                </div>
                <div className="grid gap-x-6 gap-y-1 sm:grid-cols-2 lg:grid-cols-3">
                  {nodes.map((n) => (
                    <div key={n.node} className="flex items-center justify-between gap-2 text-xs">
                      <span className="truncate text-mut">{n.node}</span>
                      <span className={`micro shrink-0 ${n.available ? "text-ok" : "text-warn"}`}>
                        {n.available ? `UP${n.inflight ? ` · ${n.inflight}` : ""}` : `${t("ad.botsCooling")}${n.blocked_for_s ? ` ${n.blocked_for_s}s` : ""}`}
                      </span>
                      <span className="micro shrink-0 text-dim">{n.ok}/{n.failed}</span>
                    </div>
                  ))}
                </div>
              </>
            )}
          </Panel>

          <Panel title={t("ad.config")}>
            <div className="grid gap-x-6 gap-y-1.5 sm:grid-cols-2">
              {Object.keys(cfg)
                .sort()
                .map((k) => (
                  <KV key={k} k={k} v={String(cfg[k] ?? "—")} />
                ))}
            </div>
          </Panel>

          <div className="flex flex-wrap justify-end gap-2">
            <Btn onClick={clearCache} disabled={clearing}>
              <Eraser size={12} aria-hidden /> {t("ad.cacheClear")}
            </Btn>
            <Btn onClick={() => mutateSys()}>
              <RefreshCw size={12} aria-hidden /> {t("common.retry")}
            </Btn>
          </div>
        </div>
      )}
    </div>
  );
}
