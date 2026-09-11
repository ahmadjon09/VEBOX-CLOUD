
const FORCE = (import.meta.env.VITE_STATUS_ONLY as string | undefined) === "1";

export function isStatusOnly(): boolean {
  if (FORCE) return true;
  if (typeof window === "undefined") return false;
  try {
    if (new URLSearchParams(window.location.search).get("status") === "1") return true;
  } catch {
  }
  return false;
}
