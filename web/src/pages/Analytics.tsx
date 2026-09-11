
import { useState } from "react";
import { useI18n } from "../i18n/context";
import { Seo } from "../components/Seo";
import { ChartFrame, TrendChart, BytesChart, ActionPie } from "../components/Charts";
import { ErrorBox, Empty, Micro, Spinner, Stat } from "../components/ui";
import { useAccount, useAnalytics } from "../services/media";
import { bytes, num } from "../utils/format";

const RANGES = [
  { d: 7, k: "an.7d" },
  { d: 30, k: "an.30d" },
  { d: 90, k: "an.90d" },
];

export function Analytics() {
  const { t } = useI18n();
  const [days, setDays] = useState(30);
  const { data, error, isLoading, mutate } = useAnalytics(days);
  const { data: account } = useAccount();

  const usage = account?.data?.account?.usage;
  const limits = account?.data?.account?.limits;

  if (isLoading) return <div className="mx-auto max-w-5xl px-4 py-8"><Spinner label={t("common.loading")} /></div>;
  if (error) return <div className="mx-auto max-w-5xl px-4 py-8"><ErrorBox text={t("err.generic")} onRetry={() => mutate()} /></div>;

  const d = data!.data;
  const timeline = d.timeline || [];
  const view = (d.view || { count: 0 }) as { count: number };
  const upload = (d.upload || { count: 0 }) as { count: number };
  const del = (d.delete || { count: 0 }) as { count: number };
  const stream = (d.stream || { count: 0 }) as { count: number };

  const pieData = [
    { action: "view", count: view.count + stream.count },
    { action: "upload", count: upload.count },
    { action: "delete", count: del.count },
  ].filter((x) => x.count > 0);

  const hasAny = pieData.length > 0 || timeline.some((x) => x.view + x.upload + x.delete > 0);

  return (
    <div className="mx-auto max-w-5xl px-4 py-8">
      <Seo title={`${t("an.title")} — VEBOX`} path="/analytics" />

      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <h1 className="text-2xl tracking-tight text-fg">{t("an.title")}</h1>
        <div className="flex border border-line">
          {RANGES.map((r) => (
            <button
              key={r.d}
              onClick={() => setDays(r.d)}
              className={`micro px-3 py-2 ${days === r.d ? "bg-fg text-ink" : "text-mut hover:text-fg"}`}
            >
              {t(r.k)}
            </button>
          ))}
        </div>
      </div>

      <div className="mb-4 grid grid-cols-2 gap-px border border-line bg-line lg:grid-cols-4">
        <div className="bg-ink px-4 py-4">
          <Micro>{t("an.views")}</Micro>
          <div className="mt-2 text-xl text-fg">{num(view.count + stream.count)}</div>
        </div>
        <div className="bg-ink px-4 py-4">
          <Micro>{t("an.uploads")}</Micro>
          <div className="mt-2 text-xl text-fg">{num(upload.count)}</div>
        </div>
        <div className="bg-ink px-4 py-4">
          <Micro>{t("an.downloads")}</Micro>
          <div className="mt-2 text-xl text-fg">{num(del.count)}</div>
        </div>
        <div className="bg-ink px-4 py-4">
          <Micro>{t("an.data")}</Micro>
          <div className="mt-2 text-xl text-fg">{bytes(timeline.reduce((s, x) => s + (x.bytes || 0), 0))}</div>
        </div>
      </div>

      {usage && limits && (
        <div className="mb-4 grid grid-cols-2 gap-px border border-line bg-line lg:grid-cols-3">
          <Stat
            k={t("an.storage")}
            v={limits.storage_mb > 0
              ? `${bytes(usage.storage_bytes)} / ${bytes(limits.storage_mb * 1024 * 1024)}`
              : bytes(usage.storage_bytes)}
          />
          <Stat k={t("an.files")} v={String(usage.file_count)} />
          <Stat
            k={`${t("an.thisMonth")}`}
            v={usage.month_files_quota > 0
              ? `${usage.month_files} / ${usage.month_files_quota}`
              : String(usage.month_files)}
          />
        </div>
      )}

      {hasAny ? (
        <>
          <div className="mb-4">
            <ChartFrame label={`${t("an.views")} · ${t("an.uploads")}`}>
              <TrendChart data={timeline} />
            </ChartFrame>
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <ChartFrame label={t("an.data")}>
              <BytesChart data={timeline} />
            </ChartFrame>
            <ChartFrame label={t("an.byType")}>
              {pieData.length > 0 ? (
                <ActionPie data={pieData} />
              ) : (
                <div className="flex h-full items-center justify-center">
                  <Micro>{t("an.empty")}</Micro>
                </div>
              )}
            </ChartFrame>
          </div>
        </>
      ) : (
        <Empty title={t("an.empty")} />
      )}
    </div>
  );
}
