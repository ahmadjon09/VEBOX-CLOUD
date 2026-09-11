
import { ChevronRight } from "lucide-react";
import { useI18n } from "../i18n/context";
import { legalSections, privacySections } from "../i18n/translations";
import { Seo } from "../components/Seo";
import { Micro } from "../components/ui";

const UPDATED = "2026-09-01";

export function Legal({ kind }: { kind: "terms" | "privacy" }) {
  const { t, lang } = useI18n();
  const title = t(kind === "terms" ? "lg.terms.title" : "lg.privacy.title");
  const sections = kind === "terms" ? legalSections(lang) : privacySections(lang);

  return (
    <div className="mx-auto max-w-2xl px-4 py-12">
      <Seo title={`${title} — VEBOX`} path={kind === "terms" ? "/terms" : "/privacy"} />

      <div className="mb-8">
        <Micro className="mb-2 text-dim">VEBOX · {t("lg.updated")}</Micro>
        <h1 className="text-3xl tracking-tight text-fg">{title}</h1>
        <div className="micro mt-3 text-mut">{UPDATED}</div>
      </div>

      <div className="space-y-8">
        {sections.map((s) => (
          <section key={s.h}>
            <h2 className="micro mb-2 flex items-center gap-1.5 text-fg">
              <ChevronRight size={12} className="text-accent2" aria-hidden /> {s.h}
            </h2>
            <p className="text-sm leading-relaxed text-mut">{s.p}</p>
          </section>
        ))}
      </div>

      <div className="mt-12 border-t border-line pt-4">
        <div className="micro text-dim">VEBOX — {t("app.tagline").toUpperCase()}</div>
      </div>
    </div>
  );
}
