
import { Link } from "react-router-dom";
import { ArrowLeft, Compass } from "lucide-react";
import { useI18n } from "../i18n/context";
import { Seo } from "../components/Seo";
import { Btn } from "../components/ui";

export function NotFound() {
  const { t } = useI18n();
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-6 px-4 text-center">
      <Seo title={t("nf.title")} path="/404" />
      <Compass size={56} className="text-dim" aria-hidden />
      <h1 className="text-2xl tracking-tight text-fg">{t("nf.title")}</h1>
      <p className="max-w-sm text-sm text-mut">{t("nf.desc")}</p>
      <Link to="/">
        <Btn variant="solid">
          <ArrowLeft size={12} aria-hidden /> {t("nf.home")}
        </Btn>
      </Link>
    </div>
  );
}
