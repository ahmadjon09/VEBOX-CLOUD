
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { makeT, type Lang } from "./translations";
import { getLang, setLang as persistLang } from "../services/api";

interface I18n {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}

const Ctx = createContext<I18n>({
  lang: "uz",
  setLang: () => {},
  t: (k) => k,
});

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setL] = useState<Lang>(() => {
    const l = getLang();
    return l === "en" || l === "ru" ? l : "uz";
  });

  const setLang = useCallback((l: Lang) => {
    persistLang(l);
    setL(l);
  }, []);

  const t = useMemo(() => makeT(lang), [lang]);

  const value = useMemo<I18n>(() => {
    if (typeof document !== "undefined") document.documentElement.lang = lang;
    return { lang, setLang, t };
  }, [lang, setLang, t]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useI18n() {
  return useContext(Ctx);
}
