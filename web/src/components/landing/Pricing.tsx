
import { Check } from "lucide-react";
import { Link } from "react-router-dom";
import { motion } from "framer-motion";
import { useI18n } from "../../i18n/context";
import { TIERS } from "../../utils/tiers";

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between text-xl">
      <span className="flex items-center gap-1.5 text-fg">
        <Check size={12} className="text-ok" aria-hidden />
        {label}
      </span>
      <span className="text-dim">{value}</span>
    </div>
  );
}

export function Pricing() {
  const { t } = useI18n();
  const tier = TIERS[0];

  return (
    <div className="mx-auto max-w-md">
      <motion.div
        whileHover={{ y: -5 }}
        transition={{ type: "spring", stiffness: 300, damping: 22 }}
        className="relative flex flex-col border border-accent bg-panel hover:shadow-[0_12px_44px_-12px_rgba(0,122,204,0.5)]"
      >
        <div className="border-b border-line px-5 py-4">
          <div className="flex items-baseline justify-between">
            <span className="text-lg text-fg">{t("land.p.free.name")}</span>
            <span className="text-sm text-fg">{tier.price}</span>
          </div>
          <div className="micro mt-1 text-mut">{t("land.p.free.d")}</div>
        </div>
        <div className="flex-1 space-y-2.5 px-5 py-4">
          <Row label={t("land.pr.file")} value={tier.fileLimit} />
          <Row label={t("land.pr.rate")} value={tier.ratePerMin} />
          <Row label={t("land.pr.storage")} value={tier.storage} />
          <Row label={t("land.pr.traffic")} value={tier.traffic} />
          <Row label={t("land.pr.files")} value={tier.filesPerMonth} />
          <Row label={t("land.pr.hd")} value={tier.hdWidth} />
          <Row label={t("land.pr.analytics")} value={t("common.ok")} />
          <Row label={t("land.pr.resize")} value={t("common.ok")} />
        </div>
        <div className="px-5 pb-5">
          <Link
            to="/signup"
            className="micro block border border-accent bg-accent py-3 text-center text-white hover:bg-accent2"
          >
            {t("land.price.cta")}
          </Link>
        </div>
      </motion.div>
    </div>
  );
}
