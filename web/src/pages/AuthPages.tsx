
import { useState } from "react";
import { ArrowRight, CircleX } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../providers/AuthProvider";
import { useI18n } from "../i18n/context";
import { Seo } from "../components/Seo";
import { LogoLarge } from "../components/Logo";
import { Btn, Field, Input, Micro } from "../components/ui";
import { apiMessage, apiUrl } from "../services/api";

function AuthShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="bg-grid">
      <div className="mx-auto flex min-h-[calc(100dvh-3.5rem)] max-w-[2000px] items-center justify-center px-4 py-16">
        <div className="w-full max-w-sm border border-line bg-panel p-8">
          <div className="mb-8 flex items-center gap-3 text-fg">
            <LogoLarge size={34} />
            <div>
              <div className="micro">VEBOX</div>
              <div className="micro mt-1 text-dim">BUILD 2.0</div>
            </div>
          </div>
          {children}
        </div>
      </div>
    </div>
  );
}

function GithubBtn({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) {
  const { t } = useI18n();
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="micro flex w-full items-center justify-center gap-2 border border-line2 bg-panel2 px-4 py-3.5 text-fg hover:border-fg disabled:opacity-40"
    >
      <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden>
        <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z" />
      </svg>
      {t("auth.github")}
    </button>
  );
}

function errMsg(code: string | undefined, t: (k: string) => string, fallback: string): string {
  switch (code) {
    case "invalid_credentials":
      return t("auth.err.badCreds");
    case "email_taken":
    case "signup_failed":
      return t("auth.err.taken");
    case "auth_rate_limited":
      return t("auth.err.limited");
    case "weak_password":
      return t("auth.pwHint");
    case "invalid_email":
      return t("auth.email");
    default:
      return fallback;
  }
}

export function Login() {
  const nav = useNavigate();
  const { t } = useI18n();
  const { login, loginWithKey, hasGithub, demo } = useAuth();
  const [email, setEmail] = useState("");
  const [pw, setPw] = useState("");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submit = async () => {
    setErr("");
    setBusy(true);
    try {
      await login(email, pw);
      nav("/console");
    } catch (e: any) {
      setErr(errMsg(e?.response?.data?.error?.code, t, apiMessage(e, t("err.generic"))));
    } finally {
      setBusy(false);
    }
  };

  const submitKey = async () => {
    setErr("");
    setBusy(true);
    try {
      await loginWithKey(key);
      nav("/console");
    } catch (e: any) {
      setErr(e?.response?.status === 401 ? t("auth.err.badKey") : apiMessage(e, t("err.generic")));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthShell>
      <Seo title={`${t("auth.login.title")} — VEBOX`} desc={t("app.tagline")} path="/login" />
      <h1 className="mb-6 text-lg tracking-tight text-fg">{t("auth.login.title")}</h1>

      <div className="space-y-3">
        {hasGithub && (
          <>
            <GithubBtn onClick={() => (window.location.href = apiUrl("/auth/github/login"))} disabled={busy} />
            <div className="flex items-center gap-3 py-1">
              <span className="h-px flex-1 bg-line" />
              <Micro>{t("auth.or")}</Micro>
              <span className="h-px flex-1 bg-line" />
            </div>
          </>
        )}
        <Field label={t("auth.email")}>
          <Input
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
          />
        </Field>
        <Field label={t("auth.password")}>
          <Input
            type="password"
            autoComplete="current-password"
            value={pw}
            onChange={(e) => setPw(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            placeholder="••••••••"
          />
        </Field>

        {err && (
          <div className="micro flex items-center gap-1.5 text-err">
            <CircleX size={12} aria-hidden /> {err}
          </div>
        )}

        <Btn variant="solid" onClick={submit} loading={busy} disabled={!email || !pw} className="w-full py-4">
          {t("auth.login.submit")} <ArrowRight size={13} aria-hidden />
        </Btn>

        <div className="pt-1">
          <div className="flex items-center gap-3 py-1">
            <span className="h-px flex-1 bg-line" />
            <Micro>{t("auth.or")}</Micro>
            <span className="h-px flex-1 bg-line" />
          </div>
          <div className="flex gap-2">
            <Input
              type="text"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && submitKey()}
              placeholder="vb_live_…"
              className="flex-1"
            />
            <Btn onClick={submitKey} disabled={busy || !key.trim()}>
              {t("auth.key.btn")}
            </Btn>
          </div>
          <div className="micro mt-2 text-dim">{t("auth.key.hint")}</div>
        </div>

        {demo && <div className="micro text-dim">{t("auth.demo")}</div>}

        <div className="pt-2 text-center">
          <Link to="/signup" className="micro text-mut hover:text-fg">
            {t("auth.toSignup")}
          </Link>
        </div>
      </div>
    </AuthShell>
  );
}

export function Signup() {
  const nav = useNavigate();
  const { t } = useI18n();
  const { signup, hasGithub } = useAuth();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [pw, setPw] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submit = async () => {
    setErr("");
    if (pw.length < 8) {
      setErr(t("auth.pwHint"));
      return;
    }
    setBusy(true);
    try {
      await signup(email, pw, name || undefined);
      nav("/console");
    } catch (e: any) {
      setErr(errMsg(e?.response?.data?.error?.code, t, apiMessage(e, t("err.generic"))));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthShell>
      <Seo title={`${t("auth.signup.title")} — VEBOX`} desc={t("app.tagline")} path="/signup" />
      <h1 className="mb-6 text-lg tracking-tight text-fg">{t("auth.signup.title")}</h1>

      <div className="space-y-3">
        {hasGithub && (
          <>
            <GithubBtn onClick={() => (window.location.href = apiUrl("/auth/github/login"))} disabled={busy} />
            <div className="flex items-center gap-3 py-1">
              <span className="h-px flex-1 bg-line" />
              <Micro>{t("auth.or")}</Micro>
              <span className="h-px flex-1 bg-line" />
            </div>
          </>
        )}
        <Field label={t("auth.name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="—" autoComplete="name" />
        </Field>
        <Field label={t("auth.email")}>
          <Input
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
          />
        </Field>
        <Field label={`${t("auth.password")} — ${t("auth.pwHint")}`}>
          <Input
            type="password"
            autoComplete="new-password"
            value={pw}
            onChange={(e) => setPw(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            placeholder="••••••••"
          />
        </Field>

        {err && (
          <div className="micro flex items-center gap-1.5 text-err">
            <CircleX size={12} aria-hidden /> {err}
          </div>
        )}

        <Btn variant="solid" onClick={submit} loading={busy} disabled={!email || pw.length < 8} className="w-full py-4">
          {t("auth.signup.submit")} <ArrowRight size={13} aria-hidden />
        </Btn>

        <div className="pt-2 text-center">
          <Link to="/login" className="micro text-mut hover:text-fg">
            {t("auth.toLogin")}
          </Link>
        </div>
      </div>
    </AuthShell>
  );
}
