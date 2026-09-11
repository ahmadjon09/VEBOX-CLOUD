import useSWR from "swr";
import { api, assetUrl, isQuotaError } from "./api";

export interface MediaUrls {
  cdn: string;
  preview: string;
  download: string;
  original?: string;
  hd?: string;
  thumbnail?: string;
}

export interface MediaColor {
  hex: string;
  pct: number;
}

export type MediaKind = "image" | "audio" | "video";

export interface MediaLink {
  id: string;
  kind: MediaKind;
  visibility: "public" | "private";
  urls: MediaUrls;
}

export interface MediaItem {
  id: string;
  filename: string;
  mime_type: string;
  kind: MediaKind;
  size_bytes: number;
  size_kb: number;
  width: number;
  height: number;
  duration?: number;
  duration_fmt?: string;
  checksum: string;
  visibility: "public" | "private";
  views: number;
  created_at: string;
  bg?: string;
  colors?: MediaColor[];
  urls: MediaUrls;
}

export interface MediaRow extends MediaItem {
  ready: boolean;
}

export interface MediaListResp {
  ok: boolean;
  data: MediaLink[];
  note?: string;
  pagination: {
    total: number;
    limit: number;
    offset: number;
  };
}

export type SortKey = "newest" | "oldest" | "largest" | "smallest" | "views" | "name";

const mediaKey = (kind: string, vis: string, q: string, sort: string, offset: number) =>
  ["media", kind, vis, q, sort, offset] as const;

async function fetchList(kind?: string, vis?: string, q?: string, sort?: SortKey, offset = 0): Promise<MediaListResp> {
  const p: Record<string, string | number> = { limit: 60, offset };
  if (kind) p.kind = kind;
  if (vis) p.visibility = vis;
  if (q) p.q = q;
  if (sort) p.sort = sort;
  const r = await api.get<MediaListResp>("/v1/images", { params: p });
  return r.data;
}

export function useMediaList(kind?: string, vis?: string, q?: string, sort?: SortKey, offset = 0) {
  return useSWR<MediaListResp, Error>(mediaKey(kind || "", vis || "", q || "", sort || "newest", offset), () =>
    fetchList(kind, vis, q, sort, offset),
    { revalidateOnFocus: false }
  );
}

export function useMedia(id: string | undefined) {
  return useSWR<MediaItem, Error>(id ? ["media-one", id] : null, async () => {
    const r = await api.get<{ ok: boolean; data: MediaItem }>(`/v1/images/${id}`);
    return r.data.data;
  });
}

export const DETAILS_BATCH = 100;

export function detailKey(ids: string[]): string {
  return Array.from(new Set(ids.filter(Boolean))).sort().slice(0, DETAILS_BATCH).join(",");
}

export async function fetchDetails(ids: string[]): Promise<Record<string, MediaItem>> {
  const uniq = Array.from(new Set(ids.filter(Boolean))).slice(0, DETAILS_BATCH);
  if (!uniq.length) return {};
  const r = await api.get<{ ok: boolean; data: MediaItem[] }>("/v1/images/details", {
    params: { ids: uniq.join(",") },
  });
  const map: Record<string, MediaItem> = {};
  for (const it of r.data.data || []) map[it.id] = it;
  return map;
}

export function useMediaDetails(ids: string[]) {
  const key = detailKey(ids);
  return useSWR<Record<string, MediaItem>, Error>(key ? ["media-details", key] : null, () => fetchDetails(ids), {
    revalidateOnFocus: false,
  });
}

export function rowsFromLinks(links: MediaLink[] | undefined, details: Record<string, MediaItem> | undefined): MediaRow[] {
  if (!links?.length) return [];
  return links.map((l) => {
    const meta = details?.[l.id];
    if (meta) return { ...meta, urls: meta.urls || l.urls, ready: true };
    return {
      id: l.id,
      filename: l.id,
      mime_type: "",
      kind: l.kind,
      size_bytes: 0,
      size_kb: 0,
      width: 0,
      height: 0,
      checksum: "",
      visibility: l.visibility,
      views: 0,
      created_at: "",
      urls: l.urls,
      ready: false,
    };
  });
}

export function useMediaPage(kind?: string, vis?: string, q?: string, sort?: SortKey, offset = 0) {
  const list = useMediaList(kind, vis, q, sort, offset);
  const links = list.data?.data;
  const details = useMediaDetails((links || []).map((l) => l.id));
  const rows = rowsFromLinks(links, details.data);
  return {
    rows,
    total: list.data?.pagination?.total ?? 0,
    isLoading: list.isLoading,
    error: list.error || details.error,
    refreshing: Boolean(links && links.length && !rows.every((r) => r.ready)),
    mutate: list.mutate,
  };
}

export interface UploadProgress {
  id: string;
  name: string;
  pct: number;
  state: "uploading" | "done" | "error";
  error?: string;
}

export async function uploadFile(
  file: File,
  onPct: (pct: number) => void,
  opts?: { visibility?: "public" | "private" }
): Promise<MediaItem> {
  const form = new FormData();
  form.append("file", file);
  if (opts?.visibility) form.append("visibility", opts.visibility);

  const r = await api.post<{ ok: boolean; data: MediaItem }>("/v1/images", form, {
    headers: { "Content-Type": "multipart/form-data" },
    onUploadProgress: (e) => {
      if (e.total) onPct(Math.round((e.loaded / e.total) * 100));
    },
  });
  return r.data.data;
}

export async function uploadAvatar(file: File): Promise<{ avatar_url: string }> {
  const form = new FormData();
  form.append("file", file);
  const r = await api.post<{ ok: boolean; data: { avatar_url: string } }>("/v1/profile/avatar", form);
  return r.data.data;
}

export async function deleteMedia(id: string): Promise<void> {
  await api.delete(`/v1/images/${id}`);
}

export async function bulkDelete(ids: string[]): Promise<void> {
  await api.delete("/v1/images", { params: { ids: ids.join(",") } });
}

export async function setVisibility(id: string, visibility: "public" | "private"): Promise<MediaItem> {
  const r = await api.patch<{ ok: boolean; data: MediaItem }>(`/v1/images/${id}`, { visibility });
  return r.data.data;
}

export async function bulkVisibility(ids: string[], visibility: "public" | "private"): Promise<void> {
  await api.patch("/v1/images", null, { params: { ids: ids.join(","), visibility } });
}

export interface SignedLink {
  url: string;
  expires_at: string;
  ttl_seconds: number;
  visibility: string;
}

export async function createSignedLink(id: string, ttlSeconds: number): Promise<SignedLink> {
  const r = await api.post<{ ok: boolean; data: SignedLink }>(`/v1/images/${id}/share`, {
    ttl_seconds: ttlSeconds,
  });
  return r.data.data;
}

export async function downloadOriginal(item: { id: string; filename: string; urls: MediaUrls }): Promise<void> {
  const r = await api.get<Blob>(`/d/${item.id}`, { responseType: "blob" });
  const url = URL.createObjectURL(r.data);
  const a = document.createElement("a");
  a.href = url;
  a.download = item.filename || item.id;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 30_000);
}

export function deliveryUrl(item: { urls: MediaUrls }): string {
  return assetUrl(item.urls.cdn || item.urls.hd || item.urls.original);
}

export interface AccountInfo {
  tier: string;
  limits: {
    tier: string;
    rate_per_min: number;
    max_upload_kb: number;
    max_audio_kb: number;
    storage_mb: number;
    monthly_files: number;
    analytics: boolean;
    max_hd_width: number;
    hd_processing: boolean;
  };
  usage: {
    storage_bytes: number;
    file_count: number;
    month_files: number;
    month_files_quota: number;
    storage_quota_bytes: number;
  };
}

export function useAccount() {
  return useSWR<{ ok: boolean; data: { user: any; account: AccountInfo } }, Error>(["account"], async () => {
    const r = await api.get("/v1/me");
    return r.data;
  });
}

export interface UserSettings {
  hd_processing: boolean;
  max_hd_width: number;
}

export async function getSettings(): Promise<UserSettings> {
  const r = await api.get<{ ok: boolean; data: UserSettings }>("/v1/settings");
  return r.data.data;
}

export async function updateSettings(patch: Partial<UserSettings>): Promise<UserSettings> {
  const r = await api.patch<{ ok: boolean; data: UserSettings }>("/v1/settings", patch);
  return r.data.data;
}

export interface AnalyticsData {
  [action: string]: { count: number; bytes: number } | { date: string; view: number; upload: number; delete: number; bytes: number }[];
  timeline: { date: string; view: number; upload: number; delete: number; bytes: number }[];
}

export interface AnalyticsResp {
  ok: boolean;
  days: number;
  pro: boolean;
  data: AnalyticsData;
}

export function useAnalytics(days: number) {
  return useSWR<AnalyticsResp, Error>(["analytics", days], async () => {
    const r = await api.get<AnalyticsResp>("/v1/analytics", { params: { days } });
    return r.data;
  });
}

export interface KeyItem {
  id: string;
  user_id: string;
  name: string;
  prefix: string;
  revoked: boolean;
  created_at: string;
  last_used_at?: string;
}

export function useKeys() {
  return useSWR<{ ok: boolean; data: KeyItem[] }, Error>(["keys"], async () => {
    const r = await api.get("/v1/keys");
    return r.data;
  });
}

export async function createKey(name: string): Promise<{ key: KeyItem; plain: string }> {
  const r = await api.post<{ ok: boolean; data: KeyItem; api_key: string }>("/v1/keys", { name });
  return { key: r.data.data, plain: r.data.api_key };
}

export async function deleteKey(id: string): Promise<void> {
  await api.delete(`/v1/keys/${id}`);
}

export async function deleteAllKeys(): Promise<void> {
  await api.delete("/v1/keys");
}

export interface StatusData {
  ok: boolean;
  status: "operational" | "degraded";
  uptime?: { seconds: number; started_at: string };
  uptime_90d?: number;
  version: string;
  storage: string;
  database: string;
  history: { date: string; total: number; up: number; uptime_pct: number }[];
  incidents: {
    id: string;
    component: string;
    summary: string;
    started_at: string;
    ended_at?: string;
  }[];
  updated_at: string;
}

export function useStatus(refreshMs = 60_000) {
  return useSWR<StatusData, Error>(["status"], async () => {
    const r = await api.get<StatusData>("/v1/status");
    return r.data;
  }, { refreshInterval: refreshMs, revalidateOnFocus: false });
}

export interface AdminUserRow {
  user: {
    id: string;
    email: string;
    name: string;
    avatar_url: string;
    provider: string;
    is_admin: boolean;
    banned: boolean;
    created_at: string;
  };
  subscription: { tier: string; expires_at?: string } | null;
  effective_policy: { tier: string; storage_mb: number; monthly_files: number; rate_per_min: number };
  usage: { storage_bytes: number; file_count: number; month_files: number } | null;
}

export function useAdminUsers(offset = 0) {
  return useSWR<{ ok: boolean; data: AdminUserRow[] }, Error>(["admin-users", offset], async () => {
    const r = await api.get("/v1/admin/users", { params: { limit: 100, offset } });
    return r.data;
  });
}

export async function adminSetTier(userId: string, tier: string): Promise<void> {
  await api.put(`/v1/admin/users/${userId}/subscription`, { tier });
}

export async function adminSetBan(userId: string, banned: boolean): Promise<void> {
  await api.post(`/v1/admin/users/${userId}/ban`, { banned });
}

export async function adminSetAdmin(userId: string, admin: boolean): Promise<void> {
  await api.post(`/v1/admin/users/${userId}/admin`, { admin });
}

export interface DbTableStat {
  name: string;
  rows: number;
  bytes: number;
}

export interface DbStats {
  engine?: string;
  connected?: boolean;
  error?: string;
  database?: string;
  server_version?: string;
  size_bytes?: number;
  data_bytes?: number;
  storage_bytes?: number;
  index_bytes?: number;
  quota_bytes?: number;
  free_bytes?: number;
  used_percent?: number;
  collections?: number;
  objects?: number;
  images?: number;
  tables?: DbTableStat[];
  databases?: { name: string; size_bytes: number }[];
  pool?: { open: number; in_use: number; idle: number; wait_count: number; max_open: number };
  note?: string;
}

export interface AdminSystem {
  ok: boolean;
  uptime: string;
  uptime_seconds: number;
  started_at: string;
  version: string;
  system: {
    go_version: string;
    os: string;
    arch: string;
    num_cpu: number;
    num_goroutine: number;
    pid: number;
    hostname: string;
    server_time: string;
    heap_alloc: number;
    heap_sys: number;
    gc_pause_ms: number;
    num_gc: number;
    disk?: { total_bytes: number; free_bytes: number; avail_bytes: number; used_bytes: number; used_percent: number };
  };
  memory: {
    state?: string;
    limit_bytes?: number;
    used_bytes?: number;
    used_percent?: number;
    heap_alloc?: number;
    sys?: number;
    num_gc?: number;
    goroutines?: number;
    heavy_slots?: number;
    heavy_inuse?: number;
    forced_releases?: number;
  };
  redis: {
    enabled: boolean;
    connected?: boolean;
    addr?: string;
    hits?: number;
    misses?: number;
    sets?: number;
    skipped?: number;
    errors?: number;
    hit_rate?: number;
    budget_bytes?: number;
    cached_bytes?: number;
    used_memory?: number;
    server_maxmemory?: number;
    policy?: string;
    meta_ttl?: string;
    variant_ttl?: string;
    max_value_bytes?: number;
    uptime?: string;
    last_error?: string;
  };
  jobs: {
    pending?: number;
    queue_len?: number;
    queue_cap?: number;
    staging?: number;
    staging_cap?: number;
    staged_bytes?: number;
    meta_cache?: number;
    dedup?: number;
  };
  storage_backend: {
    mode: string;
    nodes: {
      node: string;
      available: boolean;
      inflight: number;
      ok: number;
      failed: number;
      blocked_for_s?: number;
    }[];
    summary?: {
      nodes: number;
      available: number;
      cooling: number;
      ok: number;
      failed: number;
      inflight: number;
      link_cache?: number;
    };
  };
  databases: { postgres?: DbStats; mongo?: DbStats };
  tiers?: Record<string, any>;
  config?: Record<string, any>;
}

export function useAdminSystem(refreshMs = 20_000) {
  return useSWR<{ ok: boolean } & AdminSystem, Error>(["admin-system"], async () => {
    const r = await api.get("/v1/admin/system");
    return r.data;
  }, { refreshInterval: refreshMs, revalidateOnFocus: false });
}

export async function adminInvalidateCache(kind: "all" | "auth" | "media" | "redis" = "all"): Promise<void> {
  await api.post("/v1/admin/cache/invalidate", null, { params: kind === "all" ? {} : { kind } });
}

export function quotaFromError(err: unknown): { used: number; limit: number } | null {
  if (!isQuotaError(err)) return null;
  const b: any = (err as any)?.response?.data;
  const used = b?.error?.used ?? b?.used;
  const limit = b?.error?.limit ?? b?.limit;
  if (typeof used === "number" && typeof limit === "number") return { used, limit };
  return null;
}
