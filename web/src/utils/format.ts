
const UNITS = ["B", "KB", "MB", "GB", "TB"];

export function bytes(n: number | undefined | null): string {
  if (n == null || isNaN(n) || n < 0) return "—";
  if (n < 1024) return `${n} B`;
  let v = n;
  let i = 0;
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${UNITS[i]}`;
}

export function kb(n: number | undefined | null): string {
  if (n == null || isNaN(n)) return "—";
  return bytes(n * 1024);
}

export function num(n: number | undefined | null): string {
  if (n == null || isNaN(n)) return "—";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

export function uptimeBarClass(pct: number): string {
  if (pct >= 90) return "bg-ok/70";
  if (pct >= 75) return "bg-warn/70";
  return "bg-err/70";
}

export function statusLooksOk(status?: string, uptime90?: number): boolean {
  if (typeof uptime90 === "number" && uptime90 >= 90) return true;
  return status === "operational";
}

export function uptimeFmt(seconds: number): string {
  if (!seconds || seconds <= 0) return "0m";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (d > 0) return `${d}d ${h}h ${String(m).padStart(2, "0")}m`;
  if (h > 0) return `${h}h ${String(m).padStart(2, "0")}m`;
  if (m > 0) return `${m}m ${String(s).padStart(2, "0")}s`;
  return `${s}s`;
}

export function timeAgo(iso: string | undefined, lang: string): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (isNaN(t)) return "—";
  const diff = Date.now() - t;
  const min = Math.floor(diff / 60000);
  const hr = Math.floor(min / 60);
  const day = Math.floor(hr / 24);
  if (lang === "en") {
    if (min < 1) return "just now";
    if (min < 60) return `${min}m ago`;
    if (hr < 24) return `${hr}h ago`;
    if (day < 30) return `${day}d ago`;
    return new Date(iso).toLocaleDateString("en");
  }
  if (lang === "ru") {
    if (min < 1) return "только что";
    if (min < 60) return `${min} мин назад`;
    if (hr < 24) return `${hr} ч назад`;
    if (day < 30) return `${day} дн назад`;
    return new Date(iso).toLocaleDateString("ru");
  }
  if (min < 1) return "hozirgina";
  if (min < 60) return `${min} daq oldin`;
  if (hr < 24) return `${hr} soat oldin`;
  if (day < 30) return `${day} kun oldin`;
  return new Date(iso).toLocaleDateString("uz");
}

export function dateFmt(iso: string | undefined, lang: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return d.toLocaleDateString(lang === "en" ? "en" : lang === "ru" ? "ru" : "uz", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function durFmt(sec: number | undefined): string {
  if (!sec || sec <= 0) return "";
  const s = Math.round(sec);
  const m = Math.floor(s / 60);
  const r = s % 60;
  if (m >= 60) {
    const h = Math.floor(m / 60);
    return `${h}:${String(m % 60).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
  }
  return `${m}:${String(r).padStart(2, "0")}`;
}

export function keyMask(prefix: string | undefined): string {
  if (!prefix) return "—";
  return prefix.length > 18 ? prefix.slice(0, 15) + "…" : prefix;
}
