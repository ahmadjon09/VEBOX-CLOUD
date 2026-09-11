
import { useEffect, useRef } from "react";

export type HotkeyBinds = Record<string, (e: KeyboardEvent) => void>;

function isTyping(el: EventTarget | null): boolean {
  if (!el) return false;
  const t = el as HTMLElement;
  if (!t.tagName) return false;
  return t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable;
}

function modalOpen(): boolean {
  return !!document.querySelector("[data-vb-modal]");
}

export function useHotkeys(binds: HotkeyBinds, enabled = true) {
  const ref = useRef(binds);
  ref.current = binds;

  useEffect(() => {
    if (!enabled) return;
    const h = (e: KeyboardEvent) => {
      if (modalOpen()) return;
      const b = ref.current;

      if (e.ctrlKey || e.metaKey) {
        const key = `ctrl+${e.key.toLowerCase()}`;
        if (b[key]) {
          e.preventDefault();
          b[key](e);
        }
        return;
      }
      if (e.altKey) return;
      if (isTyping(e.target)) return;

      const fn = b[e.key];
      if (fn) {
        e.preventDefault();
        fn(e);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [enabled]);
}
