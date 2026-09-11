import { describe, it, expect } from "vitest";
import { deliveryUrl, detailKey, rowsFromLinks, type MediaItem, type MediaLink } from "../src/services/media";
import { cleanColors } from "../src/components/Palette";

const link = (id: string): MediaLink => ({
  id,
  kind: "image",
  visibility: "public",
  urls: { cdn: `/cdn/${id}`, preview: `/preview/${id}`, download: `/d/${id}` },
});

const meta = (id: string): MediaItem => ({
  ...link(id),
  filename: `${id}.png`,
  mime_type: "image/png",
  size_bytes: 4096,
  size_kb: 4,
  width: 800,
  height: 600,
  checksum: "x",
  views: 1,
  created_at: "2026-09-01T00:00:00Z",
  bg: "#101010",
  colors: [{ hex: "#101010", pct: 60 }],
});

describe("ro'yxat + details oqimi", () => {
  it("details kelguncha qator tayyor emas (ready: false)", () => {
    const rows = rowsFromLinks([link("a1")], undefined);
    expect(rows).toHaveLength(1);
    expect(rows[0].ready).toBe(false);
    expect(rows[0].urls.cdn).toBe("/cdn/a1");
    expect(rows[0].id).toBe("a1");
  });

  it("details bilan birlashganda to'liq ma'lumot chiqadi", () => {
    const rows = rowsFromLinks([link("a1")], { a1: meta("a1") });
    expect(rows[0].ready).toBe(true);
    expect(rows[0].filename).toBe("a1.png");
    expect(rows[0].size_bytes).toBe(4096);
    expect(rows[0].colors).toHaveLength(1);
  });

  it("bo'sh ro'yxat — bo'sh qatorlar", () => {
    expect(rowsFromLinks(undefined, undefined)).toEqual([]);
    expect(rowsFromLinks([], { a1: meta("a1") })).toEqual([]);
  });

  it("detailKey tartibga soladi va 100 ta bilan cheklaydi", () => {
    expect(detailKey(["b", "a"])).toBe("a,b");
    expect(detailKey([])).toBe("");
    expect(detailKey(["a", "a", "b"]).split(",")).toEqual(["a", "b"]);
    expect(detailKey(Array.from({ length: 140 }, (_, i) => `id${i}`)).split(",")).toHaveLength(100);
  });

  it("yetkazib berish havolasi — CDN (kalitsiz ishlaydigan yagona manzil)", () => {
    expect(deliveryUrl({ urls: { cdn: "/cdn/x", preview: "", download: "" } })).toContain("/cdn/x");
    expect(deliveryUrl({ urls: { cdn: "", hd: "/i/x?v=hd", preview: "", download: "" } })).toContain("/i/x");
  });
});

describe("palitra", () => {
  it("faqat #rrggbb va 8 tagacha; dublikatlar olib tashlanadi", () => {
    const input = [
      { hex: "#c87828", pct: 41.2 },
      { hex: "#C87828", pct: 20 },
      { hex: "red", pct: 10 },
      { hex: "#fff", pct: 5 },
    ];
    for (let i = 0; i < 9; i++) input.push({ hex: `#1${i}2${i}3${i}`, pct: 1 });
    const list = cleanColors(input);
    expect(list[0]).toEqual({ hex: "#c87828", pct: 41.2 });
    expect(list).toHaveLength(8);
    expect(list.map((c) => c.hex)).not.toContain("red");
  });

  it("bo'sh yoki noto'g'ri ma'lumot — bo'sh ro'yxat", () => {
    expect(cleanColors(undefined)).toEqual([]);
    expect(cleanColors([{ hex: "", pct: 0 }])).toEqual([]);
  });

  it("foiz 0..100 oralig'iga qisqartiriladi", () => {
    expect(cleanColors([{ hex: "#000000", pct: 150 }])[0].pct).toBe(100);
    expect(cleanColors([{ hex: "#000000", pct: -5 }])[0].pct).toBe(0);
    expect(cleanColors([{ hex: "#000000", pct: NaN }])[0].pct).toBe(0);
  });
});
