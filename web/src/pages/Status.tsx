
import { CircleCheck, CircleX, FileWarning } from "lucide-react";
import { useI18n } from "../i18n/context";
import { Seo } from "../components/Seo";
import { ErrorBox, Micro, Spinner } from "../components/ui";
import { useStatus } from "../services/media";
import { isStatusOnly } from "../utils/host";
import { statusLooksOk, timeAgo, uptimeBarClass, uptimeFmt } from "../utils/format";

export function Status() {
  const { t, lang } = useI18n();
  const { data, error, isLoading, mutate } = useStatus(60_000);

  if (isLoading) return <div className="mx-auto max-w-4xl px-4 py-8"><Spinner label={t("common.loading")} /></div>;
  if (error || !data) return <div className="mx-auto max-w-4xl px-4 py-8"><ErrorBox text={t("err.generic")} onRetry={() => mutate()} /></div>;

  const ok = statusLooksOk(data.status, data.uptime_90d);
  const history = data.history || [];

  return (
    <div className="mx-auto max-w-4xl px-4 py-12">
      <Seo
        title={`${t("st.title")} — VEBOX`}
        desc={ok ? t("st.operational") : t("st.degraded")}
        path="/status"
        canonical={isStatusOnly() ? `${window.location.origin}/` : undefined}
      />

      <div className={`border bg-panel p-8 ${ok ? "border-ok/40" : "border-warn/40"}`}>
        <div className="flex items-center gap-4">
          <span className={`inline-block h-3 w-3 rounded-full ${ok ? "bg-ok" : "bg-warn"}`} />
          <h1 className="text-xl text-fg sm:text-2xl">{ok ? t("st.operational") : t("st.degraded")}</h1>
        </div>
        <div className="micro mt-4 text-dim">
          {t("st.updated")}: {data.updated_at} · v{data.version}
        </div>
      </div>

      <div className="mt-6 grid gap-px border border-line bg-line sm:grid-cols-3">
        <div className="bg-ink px-5 py-4">
          <Micro>{t("st.check.core")}</Micro>
          <div className={`micro mt-2 flex items-center gap-1.5 ${ok ? "text-ok" : "text-warn"}`}>
            {ok ? <CircleCheck size={12} aria-hidden /> : <FileWarning size={12} aria-hidden />}
            {ok ? "UP" : "DEGRADED"}
          </div>
        </div>
        <div className="bg-ink px-5 py-4">
          <Micro>{t("st.check.storage")}</Micro>
          <div className="mt-2 micro text-fg">{(data.storage || "—").toUpperCase()}</div>
        </div>
        <div className="bg-ink px-5 py-4">
          <Micro>{t("st.check.database")}</Micro>
          <div className="mt-2 micro text-fg">{(data.database || "—").toUpperCase()}</div>
        </div>
      </div>

      <div className="mt-6 grid gap-px border border-line bg-line sm:grid-cols-3">
        <div className="bg-ink px-5 py-4">
          <Micro>{t("st.availability")}</Micro>
          <div className={`mt-2 text-2xl ${typeof data.uptime_90d === "number" && data.uptime_90d >= 90 ? "text-ok" : "text-fg"}`}>
            {typeof data.uptime_90d === "number" ? `${data.uptime_90d.toFixed(2)}%` : "—"}
          </div>
        </div>
        <div className="bg-ink px-5 py-4">
          <Micro>{t("st.uptime")}</Micro>
          <div className="mt-2 text-2xl text-fg">{uptimeFmt(data.uptime?.seconds || 0)}</div>
        </div>
        <div className="bg-ink px-5 py-4">
          <Micro>{t("st.started")}</Micro>
          <div className="mt-2 text-xs text-fg">{data.uptime?.started_at || "—"}</div>
        </div>
      </div>

      {history.length > 0 && (
        <div className="mt-6 border border-line bg-panel p-5">
          <Micro className="mb-4">{t("st.history")}</Micro>
          <div className="flex h-20 items-end gap-[2px]">
            {history.map((d) => (
              <div
                key={d.date}
                title={`${d.date}: ${d.uptime_pct}%`}
                className={`flex-1 ${uptimeBarClass(d.uptime_pct)}`}
                style={{ height: `${Math.max(4, d.uptime_pct)}%` }}
              />
            ))}
          </div>
          <div className="mt-3 flex justify-between">
            <Micro className="text-dim">{history[0]?.date}</Micro>
            <Micro className="text-dim">{history[history.length - 1]?.date}</Micro>
          </div>
        </div>
      )}

      <div className="mt-6 border border-line bg-panel p-5">
        <Micro className="mb-4">{t("st.incidents")}</Micro>
        {(data.incidents || []).length === 0 ? (
          <div className="micro flex items-center gap-1.5 text-mut">
            <CircleCheck size={12} aria-hidden /> {t("st.none")}
          </div>
        ) : (
          <div className="space-y-3">
            {data.incidents.map((i) => (
              <div key={i.id} className="border-l-2 border-line2 pl-4">
                <div className="flex items-center gap-3">
                  <span className={`micro flex items-center gap-1.5 ${i.ended_at ? "text-mut" : "text-warn"}`}>
                    {i.ended_at ? <CircleCheck size={11} aria-hidden /> : <CircleX size={11} aria-hidden />}
                    {i.ended_at ? "RESOLVED" : "IN PROGRESS"}
                  </span>
                  <span className="micro text-dim">{i.component}</span>
                </div>
                <div className="mt-1.5 text-xs text-fg">{i.summary}</div>
                <div className="micro mt-1 text-dim">
                  {timeAgo(i.started_at, lang)}
                  {i.ended_at ? ` → ${timeAgo(i.ended_at, lang)}` : ""}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
