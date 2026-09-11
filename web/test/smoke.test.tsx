
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, Navigate } from "react-router-dom";
import { HelmetProvider } from "react-helmet-async";
import { I18nProvider, useI18n } from "../src/i18n/context";
import { ToastProvider } from "../src/components/ui";
import { AuthProvider, useAuth } from "../src/providers/AuthProvider";
import type { ReactElement } from "react";

type RouteHandler = (url: string, method: string, body?: any, params?: any) => any;

let handler: RouteHandler = () => ({ ok: true, data: [] });

vi.mock("../src/services/api", () => {
  const LS = (k: string) => window.localStorage.getItem(k) || "";
  const setLS = (k: string, v: string) => (v ? window.localStorage.setItem(k, v) : window.localStorage.removeItem(k));
  const h: RouteHandler = (u, m, b) => (globalThis as any).__vbHandler?.(u, m, b) ?? { ok: true, data: [] };
  const resp = (url: string, method: string, body?: any, params?: any) => {
    const r = h(url, method, body, params);
    if (r && r.__error) {
      const e: any = new Error("request failed");
      e.response = { status: r.__error.status, data: r.__error.data };
      throw e;
    }
    return { data: r };
  };
  return {
    api: {
      get: (url: string, opts?: any) => resp(url, "GET", undefined, opts?.params),
      post: (url: string, body?: any, opts?: any) => resp(url, "POST", body),
      put: (url: string, body?: any) => resp(url, "PUT", body),
      patch: (url: string, body?: any, opts?: any) => resp(url, "PATCH", body, opts?.params),
      delete: (url: string, opts?: any) => resp(url, "DELETE", undefined, opts?.params),
      request: (cfg: any) => resp(cfg.url, cfg.method || "GET", cfg.data),
    },
    getToken: () => LS("vb_token"),
    setToken: (t: string) => setLS("vb_token", t),
    getApiKey: () => LS("vb_api_key"),
    setApiKey: (k: string) => setLS("vb_api_key", k),
    getLang: () => LS("vb_lang") || "uz",
    setLang: (l: string) => setLS("vb_lang", l),
    clearSession: () => { setLS("vb_token", ""); setLS("vb_api_key", ""); },
    apiMessage: (e: any, fb: string) => e?.response?.data?.error?.message || fb,
    isQuotaError: () => false,
    assetUrl: (p: string) => p || "",
    apiUrl: (p: string) => p,
    consumeHashApiKey: () => null,
  };
});

const DEMO_USER = {
  id: "usr_demo", name: "Demo", email: "demo@vebox.local",
  avatar_url: "", provider: "email", is_admin: true, banned: false,
  created_at: "2026-01-01T00:00:00Z",
};

const MEDIA_ITEM = {
  id: "k7m2xQ9a", filename: "photo.svg", mime_type: "image/svg+xml", kind: "image",
  size_bytes: 4096, size_kb: 4, width: 800, height: 600, bg: "#c87828",
  colors: [
    { hex: "#c87828", pct: 41.2 },
    { hex: "#2b3a67", pct: 26.4 },
    { hex: "#f2f0eb", pct: 18.9 },
    { hex: "#1e1e1e", pct: 13.5 },
  ],
  checksum: "abcd1234", visibility: "public", views: 42,
  created_at: "2026-09-01T10:00:00Z",
  urls: {
    original: "/i/k7m2xQ9a", hd: "/i/k7m2xQ9a?v=hd", thumbnail: "/i/k7m2xQ9a?v=thumb",
    cdn: "/cdn/k7m2xQ9a", preview: "/preview/k7m2xQ9a",
    download: "/d/k7m2xQ9a",
  },
};

const MEDIA_LINK = {
  id: MEDIA_ITEM.id, kind: MEDIA_ITEM.kind, visibility: MEDIA_ITEM.visibility,
  urls: { cdn: MEDIA_ITEM.urls.cdn, preview: MEDIA_ITEM.urls.preview, download: MEDIA_ITEM.urls.download },
};

function defaultHandler(url: string, method: string, body?: any, params?: any) {
  const u = url.replace(/^\//, "");
  if (u === "auth/session")
    return { ok: true, authenticated: (globalThis as any).__vbSession, user: (globalThis as any).__vbSession ? DEMO_USER : null, providers: ["email", "github"], demo: true };
  if (u === "v1/auth/login" && method === "POST") {
    if (body.email === "demo@vebox.local" && body.password === "demo1234")
      return { ok: true, user: DEMO_USER, token: "mock.jwt.tk" };
    return { __error: { status: 401, data: { error: { code: "invalid_credentials", message: "x" } } } };
  }
  if (u === "v1/auth/signup" && method === "POST") {
    if ((body.password || "").length < 8)
      return { __error: { status: 400, data: { error: { code: "weak_password", message: "x" } } } };
    return { ok: true, user: { ...DEMO_USER, email: body.email }, token: "mock.jwt.tk" };
  }
  if (u === "auth/logout") return { ok: true };
  if (u === "v1/settings" && method === "GET")
    return { ok: true, data: { hd_processing: (globalThis as any).__vbHd ?? true, max_hd_width: 2560 } };
  if (u === "v1/settings" && method === "PATCH") {
    (globalThis as any).__vbHd = body?.hd_processing ?? true;
    return { ok: true, data: { hd_processing: (globalThis as any).__vbHd, max_hd_width: 2560 } };
  }
  if (u === "v1/me")
    return { ok: true, data: { user: DEMO_USER, account: { tier: "pro", limits: { tier: "pro", rate_per_min: 600, max_upload_kb: 51200, max_audio_kb: 51200, storage_mb: 10240, monthly_files: 5000, analytics: true }, usage: { storage_bytes: 1000, storage_kb: 1, storage_quota_bytes: 10_000_000, file_count: 1, month_files: 1, month_files_quota: 5000 } } } };
  if (u === "v1/images" && method === "GET")
    return { ok: true, data: [MEDIA_LINK], pagination: { total: 1, limit: 60, offset: params?.offset || 0 } };
  if (u === "v1/images" && method === "POST") return { ok: true, data: MEDIA_ITEM };
  if (u === "v1/images/details" && method === "GET") return { ok: true, data: [MEDIA_ITEM] };
  if (u.startsWith("v1/images/")) {
    if (method === "GET") return { ok: true, data: MEDIA_ITEM };
    if (method === "PATCH") return { ok: true, data: { ...MEDIA_ITEM, visibility: body?.visibility || "public" } };
    if (method === "DELETE") return { ok: true, message: "O'chirildi" };
    if (u.endsWith("/share") && method === "POST")
      return { ok: true, data: { url: "/s/k7m2xQ9a?t=x", expires_at: "2026-09-07T00:00:00Z", ttl_seconds: body?.ttl_seconds || 3600, visibility: "private" } };
  }
  if (u === "v1/status")
    return { ok: true, status: "operational", uptime: { seconds: 3600, started_at: "2026-09-05T00:00:00Z" }, uptime_90d: 99.95, version: "2.0.0", storage: "vault", database: "connected", history: [{ date: "2026-09-05", total: 86400, up: 86400, uptime_pct: 100 }], incidents: [], updated_at: "2026-09-06T00:00:00Z" };
  if (u === "v1/analytics")
    return { ok: true, days: 30, pro: true, data: { view: { count: 100, bytes: 0 }, stream: { count: 10, bytes: 5000 }, upload: { count: 5, bytes: 2000 }, delete: { count: 1, bytes: 0 }, timeline: [{ date: "2026-09-01", view: 10, upload: 1, delete: 0, bytes: 100 }, { date: "2026-09-02", view: 20, upload: 2, delete: 1, bytes: 200 }] } };
  if (u === "v1/keys" && method === "GET") return { ok: true, data: [{ id: "ki_1", user_id: "usr_demo", name: "ci", prefix: "vb_live_ab12", revoked: false, created_at: "2026-08-01T00:00:00Z", last_used_at: "2026-09-05T00:00:00Z" }] };
  if (u === "v1/keys" && method === "POST") return { ok: true, data: { id: "ki_2", user_id: "usr_demo", name: body?.name || "", prefix: "vb_live_cd34", revoked: false, created_at: "2026-09-06T00:00:00Z" }, api_key: "vb_live_cd34abcdef" };
  if (u === "v1/profile" && method === "GET") return { ok: true, data: DEMO_USER };
  if (u === "v1/profile" && method === "PATCH") return { ok: true, data: { ...DEMO_USER, name: body?.name } };
  if (u === "v1/profile/avatar") return { ok: true, data: { ...DEMO_USER, avatar_url: "/i/img_av" } };
  if (u === "v1/admin/users")
    return { ok: true, data: [{ user: DEMO_USER, subscription: { tier: "free" }, effective_policy: { tier: "free", storage_mb: 512, monthly_files: 500, rate_per_min: 120 }, usage: { storage_bytes: 100, file_count: 2, month_files: 1 } }] };
  if (u.startsWith("v1/admin/users/")) return { ok: true };
  if (u === "v1/admin/system")
    return {
      ok: true, uptime: "1h", uptime_seconds: 3600, started_at: "2026-09-08T00:00:00Z", version: "2.0.0",
      system: {
        go_version: "go1.24.0", os: "linux", arch: "amd64", num_cpu: 2, num_goroutine: 24,
        pid: 1, hostname: "vebox-1", server_time: "2026-09-09T00:00:00Z",
        heap_alloc: 40 * 1024 * 1024, heap_sys: 80 * 1024 * 1024, gc_pause_ms: 3.2, num_gc: 12,
        disk: { total_bytes: 10 * 1024 ** 3, free_bytes: 6 * 1024 ** 3, avail_bytes: 6 * 1024 ** 3, used_bytes: 4 * 1024 ** 3, used_percent: 40 },
      },
      memory: { state: "ok", limit_bytes: 512 * 1024 * 1024, used_bytes: 100 * 1024 * 1024, used_percent: 19.5, heap_alloc: 40 * 1024 * 1024, sys: 80 * 1024 * 1024, goroutines: 24, heavy_slots: 2, heavy_inuse: 0, forced_releases: 1 },
      redis: { enabled: true, connected: true, addr: "rediss://redis:6379", hits: 812, misses: 88, hit_rate: 90.2, sets: 240, skipped: 3, errors: 0, budget_bytes: 30 * 1024 * 1024, cached_bytes: 12 * 1024 * 1024, used_memory: 12 * 1024 * 1024, server_maxmemory: 30 * 1024 * 1024, policy: "allkeys-lru", meta_ttl: "24h0m0s", variant_ttl: "6h0m0s" },
      jobs: { pending: 0, queue_len: 0, queue_cap: 512, staging: 0, staging_cap: 200, staged_bytes: 0, meta_cache: 34, dedup: 12 },
      storage_backend: { mode: "distributed-vault", nodes: [{ node: "node-01", available: true, inflight: 0, ok: 12, failed: 0, blocked_for_s: 0 }], summary: { nodes: 1, available: 1, cooling: 0, ok: 12, failed: 0, inflight: 0, link_cache: 3 } },
      databases: {
        postgres: { engine: "postgresql", connected: true, database: "neondb", server_version: "16.4", size_bytes: 21 * 1024 * 1024, tables: [{ name: "users", rows: 3, bytes: 40960 }], pool: { open: 4, in_use: 1, idle: 3, wait_count: 0, max_open: 20 } },
        mongo: { engine: "mongodb", connected: true, database: "liveimg", size_bytes: 40 * 1024 * 1024, data_bytes: 30 * 1024 * 1024, collections: 3, images: 12 },
      },
      tiers: {}, config: { public_url: "https://api.vebox.uz", allow_audio: true, max_audio_mb: 50, redis_enabled: true },
    };
  return { ok: true, data: [] };
}

beforeEach(() => {
  (globalThis as any).__vbHandler = (u, m, b, p) => handler(u, m, b, p);
  (globalThis as any).__vbSession = false;
  handler = defaultHandler;
});

function Providers({ children, route = "/" }: { children: ReactElement; route?: string }) {
  return (
    <MemoryRouter initialEntries={[route]}>
      <HelmetProvider>
        <I18nProvider>
          <ToastProvider>
            <AuthProvider>{children}</AuthProvider>
          </ToastProvider>
        </I18nProvider>
      </HelmetProvider>
    </MemoryRouter>
  );
}

function SessionProbe() {
  const { session, loading } = useAuth();
  return <div data-testid="probe">{loading ? "loading" : session?.user?.email || "anon"}</div>;
}

describe("auth", () => {
  it("sessiya yo'q — anon", async () => {
    render(
      <Providers>
        <SessionProbe />
      </Providers>
    );
    await waitFor(() => expect(screen.getByTestId("probe")).toHaveTextContent("anon"));
  });

  it("sessiya bor — user chiqadi", async () => {
    (globalThis as any).__vbSession = true;
    render(
      <Providers>
        <SessionProbe />
      </Providers>
    );
    await waitFor(() => expect(screen.getByTestId("probe")).toHaveTextContent("demo@vebox.local"));
  });
});

describe("sahifalar", () => {
  it("Landing: cloud API marketing (hero + kod + tariflar)", async () => {
    const { Landing } = await import("../src/pages/Landing");
    render(
      <Providers>
        <Routes>
          <Route path="/" element={<Landing />} />
        </Routes>
      </Providers>
    );
    expect(document.body.textContent).toMatch(/curl/);
    expect(document.body.textContent).toMatch(/k7m2xQ9a/);
    expect(document.body.textContent).toMatch(/v1\/images/);
    expect(document.body.textContent).toMatch(/50 MB/);
    expect(document.body.textContent).toMatch(/120/);
    expect(document.body.textContent).toMatch(/Bepul/);
    expect(document.body.textContent).not.toMatch(/10 GB/);
    expect(document.body.textContent).not.toMatch(/\bPro\b/);
    expect(document.body.textContent).toMatch(/podcast-ep12\.m4a/);
    expect(document.body.textContent).toMatch(/ep12-cover\.jpg/);
    expect(document.body.textContent).toMatch(/99\.99%/);
    expect(document.body.textContent).toMatch(/GIF/);
    expect(document.body.textContent).toMatch(/Video/);
    expect(document.body.textContent).toMatch(/MP4/);
  });

  it("Login: GitHub tugmasi chiqadi (provider sozlangan)", async () => {
    const { Login } = await import("../src/pages/AuthPages");
    render(
      <Providers route="/login">
        <Routes>
          <Route path="/login" element={<Login />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText(/GitHub/i)).toBeInTheDocument(), { timeout: 2000 });
  });

  it("Login: xato parol — xabar chiqadi", async () => {
    const { Login } = await import("../src/pages/AuthPages");
    render(
      <Providers route="/login">
        <Routes>
          <Route path="/login" element={<Login />} />
        </Routes>
      </Providers>
    );
    const email = screen.getByPlaceholderText("you@example.com");
    const pw = screen.getByPlaceholderText("••••••••");
    fireEvent.change(email, { target: { value: "demo@vebox.local" } });
    fireEvent.change(pw, { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: /Kirish|Sign in/i }));
    await waitFor(() => expect(screen.getByText(/Email yoki parol noto'g'ri/i)).toBeInTheDocument(), { timeout: 3000 });
  });

  it("Login: to'g'ri parol — sessiya o'rnadi", async () => {
    const { Login } = await import("../src/pages/AuthPages");
    render(
      <Providers route="/login">
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/console" element={<SessionProbe />} />
        </Routes>
      </Providers>
    );
    fireEvent.change(screen.getByPlaceholderText("you@example.com"), { target: { value: "demo@vebox.local" } });
    fireEvent.change(screen.getByPlaceholderText("••••••••"), { target: { value: "demo1234" } });
    fireEvent.click(screen.getByRole("button", { name: /Kirish|Sign in/i }));
    await waitFor(() => expect(screen.getByTestId("probe")).toHaveTextContent("demo@vebox.local"), { timeout: 3000 });
  });

  it("Signup: zaif parol — ogohlantirish", async () => {
    const { Signup } = await import("../src/pages/AuthPages");
    render(
      <Providers route="/signup">
        <Routes>
          <Route path="/signup" element={<Signup />} />
        </Routes>
      </Providers>
    );
    fireEvent.change(screen.getByPlaceholderText("you@example.com"), { target: { value: "new@user.uz" } });
    fireEvent.change(screen.getByPlaceholderText("••••••••"), { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: /Ro'yxatdan|Sign up/i }));
    await waitFor(() => expect(screen.getByText(/Kamida 8 belgi/i)).toBeInTheDocument(), { timeout: 3000 });
  });

  it("Status: ochiq sahifa — uptime + checks", async () => {
    const { Status } = await import("../src/pages/Status");
    render(
      <Providers route="/status">
        <Routes>
          <Route path="/status" element={<Status />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText(/Barcha tizimlar ishlayapti/i)).toBeInTheDocument(), { timeout: 3000 });
    expect(screen.getByText(/99\.95%/i)).toBeInTheDocument();
  });

  it("Docs: endpointlar ro'yxati", async () => {
    const { Docs } = await import("../src/pages/Docs");
    render(
      <Providers route="/docs">
        <Routes>
          <Route path="/docs" element={<Docs />} />
        </Routes>
      </Providers>
    );
    expect(screen.getAllByText("/v1/images").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/X-API-Key/i).length).toBeGreaterThan(0);
  });

  it("Legal: aloqa ma'lumoti YO'Q (talab)", async () => {
    const { Legal } = await import("../src/pages/Legal");
    const { container } = render(
      <Providers route="/terms">
        <Routes>
          <Route path="/terms" element={<Legal kind="terms" />} />
          <Route path="/privacy" element={<Legal kind="privacy" />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(container.textContent).toMatch(/Xizmat|The service/i), { timeout: 3000 });
    const text = container.textContent || "";
    expect(text).not.toMatch(/@example|tel:|phone|\+\d{3}/);
  });

  it("404 sahifa", async () => {
    const { NotFound } = await import("../src/pages/NotFound");
    render(
      <Providers route="/yo'q">
        <Routes>
          <Route path="*" element={<NotFound />} />
        </Routes>
      </Providers>
    );
    expect(screen.getByText(/404/i)).toBeInTheDocument();
  });
});

describe("konsol (auth)", () => {
  it("Console: grid + amallar", async () => {
    (globalThis as any).__vbSession = true;
    const { Console } = await import("../src/pages/Console");
    render(
      <Providers route="/console">
        <Routes>
          <Route path="/console" element={<Console />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("photo.svg")).toBeInTheDocument(), { timeout: 3000 });
    await waitFor(() => expect(screen.getByText("4.0 KB")).toBeInTheDocument(), { timeout: 3000 });
  });

  it("Hotkey: / tugmasi qidiruvni fokuslaydi", async () => {
    (globalThis as any).__vbSession = true;
    const { Console } = await import("../src/pages/Console");
    render(
      <Providers route="/console">
        <Routes>
          <Route path="/console" element={<Console />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("photo.svg")).toBeInTheDocument(), { timeout: 3000 });
    const search = document.querySelector('input[placeholder="Qidirish…"]');
    expect(search).not.toBeNull();
    fireEvent.keyDown(window, { key: "/" });
    expect(search).toHaveFocus();
  });

  it("Hotkey panel: /console/help route (modal-route)", async () => {
    (globalThis as any).__vbSession = true;
    const { ShortcutsHelp } = await import("../src/pages/ShortcutsHelp");
    render(
      <Providers route="/console/help">
        <Routes>
          <Route path="/console" element={<div>console</div>} />
          <Route path="/console/help" element={<ShortcutsHelp />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText(/Ctrl/)).toBeInTheDocument(), { timeout: 2000 });
    expect(screen.getByText(/Klaviatura yorliqlari/)).toBeInTheDocument();
  });

  it("MediaDetail: detal + delete route", async () => {
    (globalThis as any).__vbSession = true;
    const { MediaDetail } = await import("../src/pages/MediaDetail");
    render(
      <Providers route="/media/k7m2xQ9a">
        <Routes>
          <Route path="/media/:id/*" element={<MediaDetail />} />
          <Route path="/console" element={<div data-testid="back">console</div>} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("photo.svg")).toBeInTheDocument(), { timeout: 3000 });
    expect(screen.getByText(/photo\.svg/)).toBeInTheDocument();
  });

  it("Keys: ro'yxat + yaratish modal-route", async () => {
    (globalThis as any).__vbSession = true;
    const { KeysPage } = await import("../src/pages/Keys");
    render(
      <Providers route="/keys">
        <Routes>
          <Route path="/keys/*" element={<KeysPage />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("ci")).toBeInTheDocument(), { timeout: 3000 });
  });

  it("Profile: ism saqlash", async () => {
    (globalThis as any).__vbSession = true;
    const { ProfilePage } = await import("../src/pages/Profile");
    render(
      <Providers route="/profile">
        <Routes>
          <Route path="/profile/*" element={<ProfilePage />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("demo@vebox.local")).toBeInTheDocument(), { timeout: 3000 });
  });

  it("Profile: HD optimallashtirish toggle", async () => {
    (globalThis as any).__vbSession = true;
    (globalThis as any).__vbHd = true;
    const { ProfilePage } = await import("../src/pages/Profile");
    render(
      <Providers route="/profile">
        <Routes>
          <Route path="/profile/*" element={<ProfilePage />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("demo@vebox.local")).toBeInTheDocument(), { timeout: 3000 });
    const sw = await screen.findByRole("switch", { name: "HD optimallashtirish" });
    expect(sw.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(sw);
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("false"), { timeout: 3000 });
    expect((globalThis as any).__vbHd).toBe(false);
  });

  it("Admin: userlar + system", async () => {
    (globalThis as any).__vbSession = true;
    const { Admin } = await import("../src/pages/Admin");
    render(
      <Providers route="/admin">
        <Routes>
          <Route path="/admin" element={<Admin />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText("demo@vebox.local")).toBeInTheDocument(), { timeout: 3000 });
  });

  it("Analytics: diagramma ma'lumotlari", async () => {
    (globalThis as any).__vbSession = true;
    const { Analytics } = await import("../src/pages/Analytics");
    render(
      <Providers route="/analytics">
        <Routes>
          <Route path="/analytics" element={<Analytics />} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByText(/Analitika/i)).toBeInTheDocument(), { timeout: 3000 });
  });
});

describe("i18n", () => {
  it("til almashtirish — matn o'zgaradi", async () => {
    function LangProbe() {
      const { t, setLang } = useI18n();
      return (
        <div>
          <span data-testid="label">{t("nav.docs")}</span>
          <button onClick={() => setLang("en")}>en</button>
        </div>
      );
    }
    render(
      <Providers>
        <LangProbe />
      </Providers>
    );
    expect(screen.getByTestId("label")).toHaveTextContent("Hujjatlar");
    fireEvent.click(screen.getByRole("button", { name: "en" }));
    await waitFor(() => expect(screen.getByTestId("label")).toHaveTextContent("Docs"));
  });
});

describe("status subdomeni", () => {
  it("status rejimida boshqa sahifa ochilmaydi", async () => {
    window.history.replaceState({}, "", "/console?status=1");
    const App = (await import("../src/App")).default;
    render(<App />);
    await waitFor(() => expect(screen.getByText(/Barcha tizimlar ishlayapti/i)).toBeInTheDocument(), {
      timeout: 5000,
    });
    expect(window.location.pathname).toBe("/");
    expect(document.body.textContent).not.toMatch(/DOCS|PRIVACY/);
    window.history.replaceState({}, "", "/");
  });
});

describe("router eshigi", () => {
  it("auth yo'q — /console /login'ga yuboradi", async () => {
    function Guarded() {
      const { session, loading } = useAuth();
      if (loading) return null;
      if (!session?.user) return <Navigate to="/login" replace />;
      return <div data-testid="secret">secret</div>;
    }
    render(
      <Providers route="/console">
        <Routes>
          <Route path="/console" element={<Guarded />} />
          <Route path="/login" element={<div data-testid="login-page">login</div>} />
        </Routes>
      </Providers>
    );
    await waitFor(() => expect(screen.getByTestId("login-page")).toBeInTheDocument(), { timeout: 3000 });
  });
});
