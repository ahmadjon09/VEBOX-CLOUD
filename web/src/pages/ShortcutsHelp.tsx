
import { useNavigate } from "react-router-dom";
import { useI18n } from "../i18n/context";
import { Modal } from "../components/ui";

function Kbd({ children }: { children: string }) {
  return <kbd className="k">{children}</kbd>;
}

export function ShortcutsHelp() {
  const { t } = useI18n();
  const nav = useNavigate();
  const close = () => nav("/console");

  const rows: { keys: string[]; label: string }[] = [
    { keys: ["/"], label: t("kbd.search") },
    { keys: ["U"], label: t("kbd.upload") },
    { keys: ["Ctrl", "V"], label: t("kbd.paste") },
    { keys: ["←", "→"], label: t("kbd.nav") },
    { keys: ["Esc"], label: t("kbd.close") },
    { keys: ["?"], label: t("kbd.help") },
  ];

  return (
    <Modal title={t("kbd.title")} onClose={close}>
      <div className="space-y-1">
        {rows.map((r) => (
          <div key={r.label} className="flex items-center justify-between gap-3 border-b border-line px-1 py-2.5 last:border-0">
            <span className="text-xs text-mut">{r.label}</span>
            <span className="flex shrink-0 items-center gap-1">
              {r.keys.map((k) => (
                <Kbd key={k}>{k}</Kbd>
              ))}
            </span>
          </div>
        ))}
      </div>
      <div className="micro mt-4 text-dim">{t("kbd.note")}</div>
    </Modal>
  );
}
