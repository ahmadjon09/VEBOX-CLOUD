
import { useState } from "react";
import { Link } from "react-router-dom";
import { Menu, X } from "lucide-react";
import { Logo } from "../Logo";
import { LangSwitcher } from "../ui";
import { useI18n } from "../../i18n/context";

export function Nav({ cta, authed }: { cta: string; authed: boolean }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);

  const links = [
    { href: "#start", label: t("nav.start") },
    { href: "#code", label: t("nav.code") },
    { href: "#pricing", label: t("nav.pricing") },
    { href: "/status", label: t("nav.status"), route: true },
    { href: "/docs", label: t("nav.docs"), route: true },
  ];

  const linkCls = "micro py-1 text-mut transition-colors hover:text-accent2";

  return (
    <header className="sticky top-0 z-50 border-b border-line bg-titlebar/90 backdrop-blur-sm">
      <div className="mx-auto flex h-12 max-w-[2000px] items-center gap-3 px-4">
        <a href="#top" className="flex items-center gap-2 text-fg" aria-label="VEBOX — boshiga">
          <Logo size={22} />
          <span className="text-xs tracking-wide">VEBOX</span>
          <span className="micro hidden text-dim sm:inline">v2.0.0</span>
        </a>

        <nav className="ml-6 hidden items-center gap-6 md:flex" aria-label="Landing">
          {links.map((l) =>
            l.route ? (
              <Link key={l.href} to={l.href} className={linkCls}>
                {l.label}
              </Link>
            ) : (
              <a key={l.href} href={l.href} className={linkCls}>
                {l.label}
              </a>
            )
          )}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <LangSwitcher />
          <Link
            to={cta}
            className="micro hidden items-center border border-accent bg-accent px-4 py-2 text-white transition-colors hover:bg-accent2 sm:flex"
          >
            {authed ? t("nav.console") : t("land.hero.cta1")}
          </Link>
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            aria-label="Menu"
            className="p-1.5 text-mut transition-colors hover:text-fg md:hidden"
          >
            {open ? <X size={16} aria-hidden /> : <Menu size={16} aria-hidden />}
          </button>
        </div>
      </div>

      {open && (
        <nav className="border-t border-line bg-panel px-4 py-3 md:hidden" aria-label="Landing mobile">
          <div className="flex flex-col gap-3">
            {links.map((l) =>
              l.route ? (
                <Link key={l.href} to={l.href} onClick={() => setOpen(false)} className={linkCls}>
                  {l.label}
                </Link>
              ) : (
                <a key={l.href} href={l.href} onClick={() => setOpen(false)} className={linkCls}>
                  {l.label}
                </a>
              )
            )}
            <Link
              to={cta}
              onClick={() => setOpen(false)}
              className="micro mt-1 flex items-center justify-center border border-accent bg-accent px-4 py-2.5 text-white"
            >
              {authed ? t("nav.console") : t("land.hero.cta1")}
            </Link>
          </div>
        </nav>
      )}
    </header>
  );
}
