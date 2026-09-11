
import { Link } from "react-router-dom";
import { MotionConfig, motion } from "framer-motion";
import { ArrowRight, ShieldCheck, Sparkles, X, Zap } from "lucide-react";
import { useI18n } from "../i18n/context";
import { Seo, SchemaOrg } from "../components/Seo";
import { LogoLarge } from "../components/Logo";
import { useAuth } from "../providers/AuthProvider";
import { HeroTerminal } from "../components/landing/HeroTerminal";
import { CodeExample } from "../components/landing/CodeExample";
import { MediaWall } from "../components/landing/MediaWall";
import { StatusPanel } from "../components/landing/StatusPanel";
import { Pricing } from "../components/landing/Pricing";
import { S, Section, fadeUp, scrollIn, stagger } from "../components/landing/parts";
import { Nav } from "../components/landing/Nav";

export function Landing() {
  const { t } = useI18n();
  const { session, loading } = useAuth();
  const cta = session && !loading ? "/console" : "/signup";

  const steps = [
    { n: "01", k: "land.s1.t", d: "land.s1.d", code: <S>POST /v1/keys</S> },
    { n: "02", k: "land.s2.t", d: "land.s2.d", code: <S>POST /v1/images</S> },
    { n: "03", k: "land.s3.t", d: "land.s3.d", code: <S>GET /cdn/&#123;id&#125; · /s/&#123;id&#125;</S> },
  ];

  const feats = [
    { tk: "land.f1.t", dk: "land.f1.d" },
    { tk: "land.f2.t", dk: "land.f2.d" },
    { tk: "land.f3.t", dk: "land.f3.d" },
    { tk: "land.f4.t", dk: "land.f4.d" },
    { tk: "land.f5.t", dk: "land.f5.d" },
    { tk: "land.f6.t", dk: "land.f6.d" },
  ];

  const noyes = [
    { tk: "land.n1.t", dk: "land.n1.d" },
    { tk: "land.n2.t", dk: "land.n2.d" },
    { tk: "land.n3.t", dk: "land.n3.d" },
    { tk: "land.n4.t", dk: "land.n4.d" },
  ];

  return (
    <MotionConfig reducedMotion="user">
      <div id="top">
        <Nav cta={cta} authed={!!session && !loading} />
        <SchemaOrg />
        <Seo title="VEBOX — Media Cloud API" desc={t("land.hero.sub")} path="/" />

        <div className="bg-grid relative overflow-hidden">
          <div className="hero-glow pointer-events-none absolute inset-0" aria-hidden />
          <div className="relative mx-auto max-w-[2000px] px-4 pb-14 pt-10 sm:px-6 sm:pt-16">
            <motion.div
              variants={stagger}
              initial="hidden"
              animate="show"
              className="mb-8 flex flex-wrap items-center gap-2"
            >
              <motion.span
                variants={fadeUp}
                className="micro flex items-center gap-1.5 border border-accent bg-accent/10 px-2.5 py-1.5 text-accent2"
              >
                <Zap size={11} aria-hidden /> {t("land.mode")}
              </motion.span>
              <motion.span variants={fadeUp} className="micro border border-line px-2.5 py-1.5 text-mut">
                {t("land.chip.formats")}
              </motion.span>
              <motion.span variants={fadeUp} className="micro border border-line px-2.5 py-1.5 text-mut">
                {t("land.chip.uptime")}
              </motion.span>
              <motion.span
                variants={fadeUp}
                className="micro flex items-center gap-1.5 border border-line px-2.5 py-1.5 text-mut"
              >
                <ShieldCheck size={11} className="text-ok" aria-hidden /> HOTLINK GUARD
              </motion.span>
            </motion.div>

            <motion.div variants={stagger} initial="hidden" animate="show">
              <motion.h1 variants={fadeUp} className="text-6xl leading-none tracking-tight text-fg sm:text-7xl">
                {t("land.hero.h1")}
                {/* <span className="hero-caret" aria-hidden /> */}
                <motion.span variants={fadeUp} className="mt-3 block text-2xl tracking-tight text-accent2 sm:text-3xl">
                  {t("land.hero.h2")}
                </motion.span>
              </motion.h1>
            </motion.div>

            <div className="mt-10 grid items-start gap-8 lg:grid-cols-2">
              <motion.div
                variants={stagger}
                initial="hidden"
                animate="show"
                className="flex flex-col justify-end gap-6"
              >
                <motion.p variants={fadeUp} className="max-w-md text-sm leading-relaxed text-mut">
                  {t("land.hero.sub")}
                </motion.p>
                <motion.div variants={fadeUp} className="flex flex-wrap gap-3">
                  <motion.div whileHover={{ scale: 1.02 }} whileTap={{ scale: 0.98 }}>
                    <Link
                      to={cta}
                      className="micro flex items-center justify-center gap-2 border border-accent bg-accent px-6 py-3.5 text-white transition-colors hover:bg-accent2"
                    >
                      {t("land.hero.cta1")} <ArrowRight size={13} aria-hidden />
                    </Link>
                  </motion.div>
                  <motion.div whileHover={{ scale: 1.02 }} whileTap={{ scale: 0.98 }}>
                    <Link
                      to="/docs"
                      className="micro block border border-line2 px-6 py-3.5 text-fg transition-colors hover:border-accent2 hover:text-accent2"
                    >
                      {t("land.hero.cta2")}
                    </Link>
                  </motion.div>
                </motion.div>
                <motion.div variants={fadeUp} className="micro text-dim">
                  {t("land.hero.note")}
                </motion.div>
              </motion.div>
              <motion.div variants={fadeUp} initial="hidden" animate="show">
                <HeroTerminal />
              </motion.div>
            </div>

            <motion.div
              variants={stagger}
              initial="hidden"
              animate="show"
              className="mt-14 grid grid-cols-2 gap-px border border-line bg-line sm:grid-cols-4"
            >
              {[
                { v: "142 ms", k: "land.stat1" },
                { v: "99.99%", k: "land.stat2" },
                { v: "0", k: "land.stat3" },
                { v: "50 MB", k: "land.stat4" },
              ].map((s) => (
                <motion.div key={s.k} variants={fadeUp} className="bg-ink/80 px-5 py-4 backdrop-blur-[2px]">
                  <div className="text-lg tracking-tight text-fg">{s.v}</div>
                  <div className="micro mt-1 text-dim">{t(s.k)}</div>
                </motion.div>
              ))}
            </motion.div>
          </div>
        </div>

        <Section no="01" id="start" kicker={t("land.step.kicker")} title={t("land.step.t")}>
          <div className="space-y-px border border-line bg-line">
            {steps.map((s) => (
              <div
                key={s.n}
                className="grid gap-4 bg-ink px-6 py-7 transition-colors hover:bg-panel sm:grid-cols-[64px_1fr_auto] sm:items-center"
              >
                <div className="text-dim">{s.n}</div>
                <div>
                  <div className="micro mb-2 text-fg">{t(s.k)}</div>
                  <p className="text-xs leading-relaxed text-mut sm:text-sm">{t(s.d)}</p>
                </div>
                <code className="hidden border border-line bg-panel px-3 py-2 text-xs text-teal md:block">{s.code}</code>
              </div>
            ))}
          </div>
        </Section>

        <Section no="02" kicker={t("land.wall.kicker")} title={t("land.wall.t")}>
          <div className="grid gap-6 lg:grid-cols-[1.4fr_1fr]">
            <div>
              <MediaWall />
              <div className="micro mt-2 text-dim">{t("land.wall.cap")}</div>
            </div>
            <div>
              <StatusPanel />
              <div className="micro mt-2 text-dim">{t("land.ui.cap2")}</div>
            </div>
          </div>
        </Section>

        <Section no="03" kicker={t("land.feat.kicker")} title={t("land.feat.t")}>
          <div className="grid gap-px border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
            {feats.map((f, i) => (
              <div key={f.tk} className="bg-ink px-6 py-7 transition-colors hover:bg-panel">
                <div className="mb-3 flex items-center gap-2 text-dim">
                  <Sparkles size={12} aria-hidden />
                  {String(i + 1).padStart(2, "0")}
                </div>
                <div className="micro text-fg">{t(f.tk)}</div>
                <p className="mt-3 text-xs leading-relaxed text-mut">{t(f.dk)}</p>
              </div>
            ))}
          </div>
        </Section>

        <Section no="04" id="code" kicker={t("land.code.kicker")} title={t("land.code.t")}>
          <CodeExample />
        </Section>

        <Section no="05" kicker={t("land.nope.kicker")} title={t("land.nope.t")}>
          <div className="grid gap-px border border-line bg-line sm:grid-cols-2">
            {noyes.map((b) => (
              <div key={b.tk} className="bg-ink px-6 py-7 transition-colors hover:bg-panel">
                <div className="mb-3 text-err">
                  <X size={14} aria-hidden />
                </div>
                <div className="micro text-fg">{t(b.tk)}</div>
                <p className="mt-3 text-xs leading-relaxed text-mut">{t(b.dk)}</p>
              </div>
            ))}
          </div>
        </Section>

        <Section no="06" id="pricing" kicker={t("land.price.kicker")} title={t("land.price.t")}>
          <Pricing />
          <div className="micro mt-5 text-dim">{t("land.price.note")}</div>
        </Section>

        <motion.section {...scrollIn} className="border-t border-line">
          <div className="mx-auto flex max-w-[2000px] flex-col items-start gap-8 px-4 py-20 sm:px-6">
            <div className="flex items-center gap-4">
              <div className="hidden text-teal sm:block">
                <LogoLarge size={56} />
              </div>
              <div>
                <h2 className="text-3xl tracking-tight text-fg">{t("land.cta.t")}</h2>
                <p className="mt-2 max-w-lg text-sm leading-relaxed text-mut">{t("land.cta.d")}</p>
              </div>
            </div>
            <motion.div whileHover={{ scale: 1.03 }} whileTap={{ scale: 0.97 }}>
              <Link
                to={cta}
                className="micro flex items-center justify-center gap-2 border border-accent bg-accent px-8 py-4 text-white hover:bg-accent2"
              >
                {t("land.cta.btn")} <ArrowRight size={13} aria-hidden />
              </Link>
            </motion.div>
          </div>
        </motion.section>
      </div>
    </MotionConfig>
  );
}
