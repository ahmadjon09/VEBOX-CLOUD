
import { useEffect, useState } from "react";
import { Check, Copy, TerminalSquare } from "lucide-react";
import { motion } from "framer-motion";
import { useI18n } from "../../i18n/context";
import { VsMiniStatus, VsWindow } from "../VsWindow";
import { copyText } from "../ui";
import { fadeUp, B, S, N } from "./parts";

const HERO_CMD = "curl -s -F file=@banner-2026.jpg https://api.vebox.uz/v1/images";

function useTypeOnce(text: string) {
  const [count, setCount] = useState(0);

  useEffect(() => {
    let alive = true;
    let t: ReturnType<typeof setTimeout>;
    const start = setTimeout(() => {
      let i = 0;
      const tick = () => {
        if (!alive) return;
        i += 1;
        setCount(i);
        if (i < text.length) t = setTimeout(tick, 16 + Math.random() * 34);
      };
      tick();
    }, 800);
    return () => {
      alive = false;
      clearTimeout(start);
      clearTimeout(t);
    };
  }, [text]);

  return { out: text.slice(0, count), done: count >= text.length };
}

export function HeroTerminal() {
  const { t } = useI18n();
  const { out, done } = useTypeOnce(HERO_CMD);
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    if (await copyText(`curl -s -F file=@banner-2026.jpg -H "X-API-Key: $KEY" https://api.vebox.uz/v1/images`)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    }
  };

  return (
    <VsWindow title={t("land.term.title")} className="group relative">
      <button
        onClick={copy}
        title={t("land.term.copy")}
        aria-label={t("land.term.copy")}
        className="absolute right-4 top-1 z-10 flex h-7 w-7 items-center justify-center border border-line bg-panel text-mut opacity-0 transition-opacity hover:text-fg group-hover:opacity-100"
      >
        {copied ? <Check size={13} /> : <Copy size={13} />}
      </button>
      <div className="p-4 text-[12.5px] leading-6">
        <div className="whitespace-pre-wrap break-all">
          <span className="flex items-start gap-2">
            <TerminalSquare size={13} className="mt-1 shrink-0 text-tok-c" aria-hidden />
            <span>
              {out.split(/(https?:\/\/\S+)/).map((p, i) =>
                i % 2 === 1 ? (
                  <span key={i} className="text-tok-b">
                    {p}
                  </span>
                ) : (
                  <span key={i}>{p}</span>
                )
              )}
              {/* <span className="cursor-blink ml-px inline-block h-[15px] w-[7px] translate-y-[2px] bg-fg/80" /> */}
            </span>
          </span>
        </div>
        {done && (
          <motion.div
            initial="hidden"
            animate="show"
            variants={{ show: { transition: { staggerChildren: 0.16, delayChildren: 0.2 } } }}
          >
            <motion.div variants={fadeUp} className="micro mt-2 text-dim">
              {t("land.term.req")}
            </motion.div>
            <motion.div variants={fadeUp}>
              <B>{"{"}</B> <B>"id"</B>: <S>"k7m2xQ9a"</S>, <B>"bg"</B>: <S>"#c87828"</S>, <B>"size_bytes"</B>: <N>248301</N>,
            </motion.div>
            <motion.div variants={fadeUp}>
              {"  "} <B>"visibility"</B>: <S>"public"</S>, <B>"kind"</B>: <S>"image"</S>,
            </motion.div>
            <motion.div variants={fadeUp}>
              {"  "} <B>"urls"</B>: {"{"} <B>"cdn"</B>: <S>"https://api.vebox.uz/cdn/k7m2xQ9a"</S> {"}"}
            </motion.div>
            <motion.div variants={fadeUp}>{"}"}</motion.div>
            <motion.div variants={fadeUp} className="flex items-center gap-1.5 text-ok">
              <Check size={13} aria-hidden /> 201 Created · 142 ms
            </motion.div>
            <motion.div variants={fadeUp} className="text-tok-c">
              ${" "}
              <span className="cursor-blink inline-block h-[15px] w-[7px] translate-y-[2px] bg-fg/60" />
            </motion.div>
          </motion.div>
        )}
      </div>
      <VsMiniStatus />
    </VsWindow>
  );
}
