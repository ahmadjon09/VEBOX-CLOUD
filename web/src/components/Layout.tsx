
import { useEffect, useState } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import {
  Activity,
  CircleCheck,
  CircleSlash,
  KeyRound,
  LogOut,
  Settings,
  UserRound,
  Files,
  ChevronDown,
} from "lucide-react";
import { useAuth } from "../providers/AuthProvider";
import { useAccount } from "../services/media";
import { assetUrl } from "../services/api";
import { useI18n } from "../i18n/context";
import { LangSwitcher } from "./ui";
import { Logo } from "./Logo";

function fileOf(path: string): string {
  const map: [string, string][] = [
    ["/console", "files.tsx"],
    ["/media/", "media.tsx"],
    ["/analytics", "analytics.tsx"],
    ["/keys", "keys.tsx"],
    ["/profile", "profile.tsx"],
    ["/admin", "admin.tsx"],
    ["/login", "login.tsx"],
    ["/signup", "signup.tsx"],
    ["/docs", "api.md"],
    ["/status", "status.json"],
    ["/terms", "terms.md"],
    ["/privacy", "privacy.md"],
  ];
  for (const [pre, name] of map) if (path.startsWith(pre)) return name;
  return "landing.tsx";
}

function UserMenu() {
  const { session, logout } = useAuth();
  const nav = useNavigate();
  const [open, setOpen] = useState(false);
  const u = session?.user;
  if (!u) return null;

  const initials = (u.name || u.email).slice(0, 2).toUpperCase();

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex items-center gap-2 border border-transparent px-2 py-1 hover:border-line hover:bg-panel2"
      >
        {u.avatar_url ? (
          <img src={assetUrl(u.avatar_url)} alt="" className="h-5 w-5 rounded-sm object-cover" />
        ) : (
          <span className="flex h-5 w-5 items-center justify-center bg-accent text-[10px] text-white">{initials}</span>
        )}
        <span className="micro hidden text-fg sm:inline">{u.name || u.email}</span>
        <ChevronDown size={11} className="text-mut" aria-hidden />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-50 w-56 border border-line2 bg-panel2 shadow-xl">
            <div className="border-b border-line px-4 py-3">
              <div className="text-xs text-fg">{u.name || "—"}</div>
              <div className="mt-1 text-[10px] text-mut">{u.email}</div>
            </div>
            <div className="p-1">
              <Link
                to="/profile"
                onClick={() => setOpen(false)}
                className="micro flex w-full items-center gap-2 px-3 py-2.5 text-left text-mut hover:bg-ink hover:text-fg"
              >
                <UserRound size={12} aria-hidden /> Profil
              </Link>
              <button
                onClick={async () => {
                  setOpen(false);
                  await logout();
                  nav("/");
                }}
                className="micro flex w-full items-center gap-2 px-3 py-2.5 text-left text-err hover:bg-err/10"
              >
                <LogOut size={12} aria-hidden /> Chiqish
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

const TABS = [
  { to: "/console", icon: Files, name: "files.tsx" },
  { to: "/analytics", icon: Activity, name: "analytics.tsx" },
  { to: "/keys", icon: KeyRound, name: "keys.tsx" },
  { to: "/profile", icon: UserRound, name: "profile.tsx" },
];

function EditorTabs({ admin }: { admin: boolean }) {
  const tabs = admin ? [...TABS, { to: "/admin", icon: Settings, name: "admin.tsx" }] : TABS;
  return (
    <div className="hidden border-b border-line bg-panel lg:flex">
      <div className="flex">
        {tabs.map((t) => (
          <NavLink
            key={t.to}
            to={t.to}
            className={({ isActive }) =>
              `tab-item flex items-center gap-2 border-r border-line px-4 py-2 text-xs ${
                isActive
                  ? "bg-ink text-fg shadow-[inset_0_2px_0_0_var(--color-accent)]"
                  : "bg-panel text-mut hover:bg-panel2 hover:text-fg"
              }`
            }
          >
            <t.icon size={12} aria-hidden />
            {t.name}
          </NavLink>
        ))}
      </div>
    </div>
  );
}

function Clock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 30000);
    return () => clearInterval(id);
  }, []);
  const hh = String(now.getHours()).padStart(2, "0");
  const mm = String(now.getMinutes()).padStart(2, "0");
  return <span className="opacity-90">{hh}:{mm}</span>;
}

function StatusBar({ user }: { user: boolean }) {
  const { t } = useI18n();
  const { data: account } = useAccount();
  const { session } = useAuth();
  const tier = account?.data?.account?.tier || session?.user?.provider;
  return (
    <div className="hidden items-center justify-between bg-accent px-3 py-1 text-[11px] text-white lg:flex">
      <div className="flex items-center gap-4">
        <span className="flex items-center gap-1.5">
          <Logo size={11} /> vebox
        </span>
        <span className="flex items-center gap-1 opacity-80">
          <CircleCheck size={12} aria-hidden /> synced
        </span>
        <span className="flex items-center gap-1 opacity-80">
          <CircleSlash size={12} aria-hidden /> 0 0
        </span>
        {user && <span className="opacity-80">{t("nav.console")}</span>}
      </div>
      <div className="flex items-center gap-4">
        <Link to="/status" className="flex items-center gap-1.5 opacity-90 transition-opacity hover:opacity-100">
          <span className="inline-block h-1.5 w-1.5 rounded-full bg-white" aria-hidden />
          99.99%
        </Link>
        <span className="opacity-80">Ln 1, Col 1</span>
        <span className="opacity-80">UTF-8</span>
        {tier && <span className="uppercase">{tier}</span>}
        <span>v2.0.0</span>
        <Clock />
      </div>
    </div>
  );
}

const MOBILE_TABS = [
  { to: "/console", icon: Files, k: "nav.files" },
  { to: "/analytics", icon: Activity, k: "nav.analytics" },
  { to: "/keys", icon: KeyRound, k: "nav.keys" },
  { to: "/profile", icon: UserRound, k: "nav.profile" },
];

export function Layout({ user = false, statusOnly = false }: { user?: boolean; statusOnly?: boolean }) {
  const { session } = useAuth();
  const { t } = useI18n();
  const location = useLocation();
  const bare = !user && location.pathname === "/" && !statusOnly;

  return (
    <div className="flex min-h-dvh flex-col bg-ink">
      {!bare && (
      <div className="sticky top-0 z-50 border-b border-line bg-titlebar">
        <div className="flex h-11 items-center">
          <div className="flex items-center gap-2 px-3">
            <div className="hidden items-center gap-1.5 sm:flex" aria-hidden>
              <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
              <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
              <span className="h-3 w-3 rounded-full bg-[#28c840]" />
            </div>
            <Link to="/" className="ml-1 flex items-center gap-2 text-fg">
              <Logo size={16} className="text-teal" />
              <span className="text-xs">VEBOX</span>
            </Link>
          </div>

          <div className="micro hidden flex-1 text-center text-mut md:block">
            {statusOnly ? "status.json" : fileOf(location.pathname)} — vebox
          </div>
          <div className="flex-1 md:hidden" />

          <div className="flex items-center gap-2 pr-3">
            <LangSwitcher />
            {statusOnly ? null : session?.user ? (
              <div className="flex items-center gap-2">
                <UserMenu />
                <Link
                  to="/console"
                  className="micro hidden border border-accent bg-accent px-3 py-1.5 text-white hover:bg-accent2 md:block"
                >
                  {t("nav.console")}
                </Link>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <Link to="/login" className="micro hidden text-mut hover:text-fg sm:block">
                  {t("nav.login")}
                </Link>
                <Link
                  to="/signup"
                  className="micro border border-accent bg-accent px-3 py-1.5 text-white hover:bg-accent2"
                >
                  {t("nav.signup")}
                </Link>
              </div>
            )}
          </div>
        </div>

        {user && <EditorTabs admin={!!session?.user?.is_admin} />}
      </div>
      )}

      <main className="flex-1 pb-24 lg:pb-0">
        <Outlet />
      </main>

      {user && (
        <nav className="pb-safe fixed inset-x-0 bottom-0 z-50 border-t border-line bg-[#333333] lg:hidden">
          <div className="grid grid-cols-4">
            {MOBILE_TABS.map((x) => (
              <NavLink
                key={x.to}
                to={x.to}
                className={({ isActive }) =>
                  `flex flex-col items-center gap-1 border-t-2 py-2.5 ${
                    isActive ? "border-accent text-fg" : "border-transparent text-mut"
                  }`
                }
              >
                <x.icon size={15} aria-hidden />
                <span className="micro">{t(x.k)}</span>
              </NavLink>
            ))}
          </div>
        </nav>
      )}

      {user ? (
        <div className="sticky bottom-0 z-40 lg:sticky lg:bottom-auto">
          <StatusBar user />
        </div>
      ) : (
        <footer className="border-t border-line bg-panel">
          <div className="mx-auto max-w-[2000px] px-4 py-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="micro flex items-center gap-2 text-dim">
                <Logo size={16} /> VEBOX v2.0 — {t("app.tagline").toUpperCase()}
              </div>
              {!statusOnly && (
                <div className="flex items-center gap-5">
                  <Link to="/docs" className="micro text-mut hover:text-fg">
                    DOCS
                  </Link>
                  <Link to="/status" className="micro text-mut hover:text-fg">
                    STATUS
                  </Link>
                  <Link to="/terms" className="micro text-mut hover:text-fg">
                    TERMS
                  </Link>
                  <Link to="/privacy" className="micro text-mut hover:text-fg">
                    PRIVACY
                  </Link>
                </div>
              )}
            </div>
            <div className="mt-4 border-t border-line pt-3">
              <div className="micro text-dim">
                MODE: {t("land.mode")} — ACCESS: {t("land.access")} — BUILD: 2.0
              </div>
            </div>
          </div>
        </footer>
      )}
    </div>
  );
}
