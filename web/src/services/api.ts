
import axios, { AxiosError } from "axios";

const API_BASE: string = (import.meta.env.VITE_API_BASE as string | undefined) ?? "https://api.vebox.uz";

export function apiUrl(path: string): string {
  return API_BASE + path;
}

const K_TOKEN = "vb_token";
const K_KEY = "vb_api_key";
const K_LANG = "vb_lang";

export function getToken(): string {
  return localStorage.getItem(K_TOKEN) || "";
}
export function setToken(t: string) {
  if (t) localStorage.setItem(K_TOKEN, t);
  else localStorage.removeItem(K_TOKEN);
}

export function getApiKey(): string {
  return localStorage.getItem(K_KEY) || "";
}
export function setApiKey(k: string) {
  if (k) localStorage.setItem(K_KEY, k);
  else localStorage.removeItem(K_KEY);
}

export function getLang(): string {
  return localStorage.getItem(K_LANG) || "uz";
}
export function setLang(l: string) {
  localStorage.setItem(K_LANG, l);
}

export function clearSession() {
  setToken("");
  setApiKey("");
}

export function assetUrl(p: string | undefined | null): string {
  if (!p) return "";
  if (/^https?:\/\//.test(p) || p.startsWith("data:")) return p;
  return API_BASE + p;
}

export const api = axios.create({
  baseURL: API_BASE,
  withCredentials: API_BASE === "",
  timeout: 120_000,
});

api.interceptors.request.use((cfg) => {
  const key = getApiKey();
  const tok = getToken();
  if (key) cfg.headers.set("X-API-Key", key);
  if (tok) cfg.headers.set("Authorization", `Bearer ${tok}`);
  return cfg;
});

api.interceptors.response.use(
  (r) => r,
  (err: AxiosError) => {
    if (err.response?.status === 401 && typeof window !== "undefined") {
      window.dispatchEvent(new CustomEvent("vb:unauthorized"));
    }
    return Promise.reject(err);
  }
);

export interface ApiErrorBody {
  error?: { code?: string; message?: string; status?: number };
  ok?: boolean;
}

export function apiMessage(err: unknown, fallback: string): string {
  if (axios.isAxiosError(err)) {
    const b = (err.response?.data || {}) as ApiErrorBody;
    if (b.error?.message) return b.error.message;
    const raw = err.response?.data as any;
    if (raw?.error?.message) return raw.error.message;
  }
  return fallback;
}

export function isQuotaError(err: unknown): boolean {
  if (!axios.isAxiosError(err)) return false;
  const s = err.response?.status;
  const code = (err.response?.data as any)?.error?.code;
  return s === 402 || code === "storage_quota_exceeded" || code === "monthly_limit_exceeded" || code === "file_too_large";
}

export function consumeHashApiKey(): string | null {
  if (typeof window === "undefined") return null;
  const p = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  const key = p.get("api_key");
  const tok = p.get("token");
  if (tok) setToken(tok);
  if (key) setApiKey(key);
  if (key || tok) {
    p.delete("api_key");
    p.delete("token");
    const rest = p.toString();
    window.history.replaceState(null, "", window.location.pathname + window.location.search + (rest ? "#" + rest : ""));
  }
  return key;
}
