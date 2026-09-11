
import { Activity, CircleCheck, CircleAlert, Radio } from "lucide-react";
import { useI18n } from "../../i18n/context";
import { useStatus } from "../../services/media";
import { statusLooksOk, uptimeBarClass, uptimeFmt } from "../../utils/format";
import { VsMiniStatus, VsWindow } from "../VsWindow";

export function StatusPanel() {
  const { t } = useI18n();
  const { data } = useStatus();
  const ok = statusLooksOk(data?.status, data?.uptime_90d);
  const hist = (data?.history || []).slice(-45);
  const incidents = data?.incidents || [];
  const openInc = incidents.filter((i) => !i.ended_at).length;

  return (
    <VsWindow title="status.vebox.uz">
      <div className="p-4">
        <div className="flex items-center gap-2">
          {data ? (
            ok ? (
              <CircleCheck size={14} className="text-ok" aria-hidden />
            ) : (
              <CircleAlert size={14} className="text-warn" aria-hidden />
            )
          ) : (
            <Radio size={14} className="animate-pulse text-mut" aria-hidden />
          )}
          <span className="text-xs text-fg">
            {data ? (ok ? t("st.operational") : t("st.degraded")) : t("st.offline")}
          </span>
        </div>

        <div className="mt-3 grid grid-cols-3 gap-1.5">
          <div className="border border-line bg-ink px-2 py-1.5">
            <div className="micro text-dim">90 D</div>
            <div className={`text-sm ${typeof data?.uptime_90d === "number" && data.uptime_90d >= 90 ? "text-ok" : "text-fg"}`}>
              {typeof data?.uptime_90d === "number" ? `${data.uptime_90d.toFixed(2)}%` : "—"}
            </div>
          </div>
          <div className="border border-line bg-ink px-2 py-1.5">
            <div className="micro text-dim">
              <Activity size={9} className="mr-1 inline" aria-hidden />
              UPTIME
            </div>
            <div className="text-sm text-fg">{data ? uptimeFmt(data.uptime?.seconds || 0) : "—"}</div>
          </div>
          <div className="border border-line bg-ink px-2 py-1.5">
            <div className="micro text-dim">BUILD</div>
            <div className="text-sm text-fg">v{data?.version || "—"}</div>
          </div>
        </div>

        <div className="mt-3 flex h-12 items-end gap-px" aria-hidden>
          {hist.length > 0 ? (
            hist.map((d) => (
              <div
                key={d.date}
                title={`${d.date}: ${d.uptime_pct}%`}
                className={`flex-1 ${uptimeBarClass(d.uptime_pct)}`}
                style={{ height: `${Math.max(8, d.uptime_pct)}%` }}
              />
            ))
          ) : (
            Array.from({ length: 45 }, (_, i) => (
              <div key={i} className="flex-1 animate-pulse bg-panel2" style={{ height: "100%" }} />
            ))
          )}
        </div>

        <div className="mt-2 flex items-center gap-2 text-[10px]">
          <span className="micro text-dim">INCIDENTS</span>
          <span className="text-mut">
            {data
              ? `${incidents.length - openInc} resolved · ${openInc} in progress`
              : "—"}
          </span>
        </div>
        <div className="micro mt-3 border-t border-line pt-2 text-dim">{t("land.statusline.d")}</div>
      </div>
      <VsMiniStatus />
    </VsWindow>
  );
}
