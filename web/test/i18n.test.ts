import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { DICT_KEYS, LANGS, makeT } from "../src/i18n/translations";

const SRC = join(process.cwd(), "src");

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) return sourceFiles(p);
    return /\.(ts|tsx)$/.test(name) ? [p] : [];
  });
}

const files = sourceFiles(SRC).filter((f) => !f.endsWith("translations.ts"));
const code = files.map((f) => readFileSync(f, "utf8")).join("\n");
const dict = DICT_KEYS();
const byKey = new Map(dict.map((d) => [d.key, d.value]));
const prefixes = new Set(dict.map((d) => d.key.split(".")[0]));

const quoted = [...code.matchAll(/"([a-z][a-z0-9_.]{1,40})"/g)].map((m) => m[1]);
const FILE_EXT = /\.(tsx?|jsx?|json|svg|png|jpe?g|webp|mp3|m4a|txt|md|css|html)$/;
const i18nLike = new Set(
  quoted.filter((k) => prefixes.has(k.split(".")[0]) && k.includes(".") && !FILE_EXT.test(k))
);

describe("i18n qamlovi", () => {
  it("har bir UI matni uchun kalit mavjud (uz/en/ru)", () => {
    const missing: string[] = [];
    for (const key of i18nLike) {
      const e = byKey.get(key);
      if (!e) {
        missing.push(key + " (yo'q)");
        continue;
      }
      for (const l of LANGS) {
        if (!e[l.code] || !e[l.code].trim()) missing.push(`${key}.${l.code} (bo'sh)`);
      }
    }
    expect(missing.sort()).toEqual([]);
  });

  it("ishlatilmagan kalit qolmagan", () => {
    const dead = dict.filter((d) => !new RegExp(`"${d.key}"`).test(code)).map((d) => d.key);
    expect(dead.sort()).toEqual([]);
  });

  it("uchala tilda ham bir xil sonli kalit", () => {
    for (const l of LANGS) {
      const t = makeT(l.code);
      for (const d of dict) {
        expect(t(d.key), `${d.key}/${l.code}`).not.toBe(d.key);
      }
    }
  });

  it("shablon o'zgaruvchilari uchala tilda bir xil to'plam", () => {
    const templated = dict.filter((d) => /\{\{\w+\}\}/.test(d.value.uz));
    expect(templated.length).toBeGreaterThan(0);
    const names = (v: string) => (v.match(/\{\{\w+\}\}/g) || []).sort().join(",");
    for (const d of templated) {
      expect(names(d.value.en), d.key).toBe(names(d.value.uz));
      expect(names(d.value.ru), d.key).toBe(names(d.value.uz));
    }
  });
});
