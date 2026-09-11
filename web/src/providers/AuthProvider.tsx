
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import useSWR, { mutate } from "swr";
import { api, clearSession, consumeHashApiKey, getApiKey, getToken, setApiKey, setToken } from "../services/api";

export interface SessionUser {
  id: string;
  name: string;
  email: string;
  avatar_url: string;
  provider: string;
  is_admin: boolean;
  banned: boolean;
  created_at: string;
}

export interface Session {
  user: SessionUser | null;
  token: string | null;
  providers: string[];
  demo: boolean;
}

interface AuthCtx {
  session: Session | null;
  loading: boolean;
  hasGithub: boolean;
  demo: boolean;
  refresh: () => void;
  login: (email: string, password: string) => Promise<Session>;
  signup: (email: string, password: string, name?: string) => Promise<Session>;
  loginWithKey: (key: string) => Promise<Session>;
  logout: () => Promise<void>;
}

const Ctx = createContext<AuthCtx | null>(null);

async function fetcher<T>(url: string, method = "GET", body?: unknown): Promise<T> {
  const r = await api.request({ url, method, data: body });
  return r.data as T;
}

function parseSession(b: any): Session {
  return {
    user: b?.user || null,
    token: null,
    providers: b?.providers || ["email"],
    demo: !!b?.demo,
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [credsPresent] = useState(() => {
    consumeHashApiKey();
    return hasCreds();
  });

  useEffect(() => {
    const h = () => {
      clearSession();
      mutate(["session"], undefined, { revalidate: false });
      mutate(["me"], undefined, { revalidate: false });
    };
    window.addEventListener("vb:unauthorized", h);
    return () => window.removeEventListener("vb:unauthorized", h);
  }, []);

  const { data, error, isLoading, mutate: m } = useSWR<Session, Error>(
    ["session"],
    async () => parseSession(await fetcher<any>("/auth/session")),
    {
      revalidateOnFocus: false,
      refreshInterval: 5 * 60_000,
      shouldRetryOnError: (err: any) => err?.response?.status !== 401,
    }
  );

  const { data: meData, isLoading: meLoading } = useSWR<{ ok: boolean; data: { user: SessionUser } }, Error>(
    credsPresent && !data?.user ? ["me"] : null,
    async () => {
      const r = await api.get("/v1/me");
      return r.data;
    },
    {
      revalidateOnFocus: false,
      refreshInterval: 5 * 60_000,
      shouldRetryOnError: (err: any) => err?.response?.status !== 401,
    }
  );

  const meSession: Session | null =
    meData?.data?.user
      ? { user: meData.data.user, token: null, providers: data?.providers || ["email"], demo: !!data?.demo }
      : null;

  useEffect(() => {
    if (!isLoading && (!meLoading || !credsPresent || !!data?.user)) setReady(true);
  }, [isLoading, meLoading, credsPresent, data]);

  const value: AuthCtx = {
    session: data?.user ? data : meSession,
    loading: isLoading || (credsPresent && meLoading && !data?.user) || !ready,
    hasGithub: (data?.providers || []).includes("github"),
    demo: data?.demo || false,
    refresh: () => m(),
    async login(email, password) {
      const b = await fetcher<any>("/v1/auth/login", "POST", { email, password });
      if (b?.token) setToken(b.token);
      const s = parseSession(b);
      await m(s, { revalidate: false });
      return s;
    },
    async signup(email, password, name) {
      const b = await fetcher<any>("/v1/auth/signup", "POST", { email, password, name });
      if (b?.token) setToken(b.token);
      const s = parseSession(b);
      await m(s, { revalidate: false });
      return s;
    },
    async loginWithKey(key) {
      const k = key.trim();
      if (!k) throw new Error("empty key");
      setApiKey(k);
      const b = await fetcher<any>("/v1/me");
      const user = b?.data?.user;
      if (!user) throw new Error("bad key");
      await m({ user, token: null, providers: ["email"], demo: false }, { revalidate: false });
      return { user, token: null, providers: ["email"], demo: false };
    },
    async logout() {
      try {
        await fetcher<any>("/auth/logout", "POST");
      } catch {
      }
      clearSession();
      await m(undefined, { revalidate: false });
      mutate(["me"], undefined, { revalidate: false });
    },
  };

  void error;

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error("useAuth AuthProvider'ichida bo'lishi kerak");
  return c;
}

export function useIsAdmin(): boolean {
  const { session } = useAuth();
  return !!session?.user?.is_admin;
}

export function hasCreds(): boolean {
  return !!getApiKey() || !!getToken();
}

export function currentKey(): string {
  return getApiKey();
}
export { setApiKey };
