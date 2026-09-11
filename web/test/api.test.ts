
import { describe, it, expect } from "vitest";
import { apiUrl, consumeHashApiKey, getApiKey, getToken } from "../src/services/api";
import { statusLooksOk, uptimeBarClass } from "../src/utils/format";

describe("apiUrl", () => {
  it("API asosini yo'lga qo'shadi (alohida domen)", () => {
    expect(apiUrl("/auth/github/login")).toBe("https://api.vebox.uz/auth/github/login");
    expect(apiUrl("/openapi.json")).toBe("https://api.vebox.uz/openapi.json");
  });
});

describe("consumeHashApiKey", () => {
  it("#api_key va #token fragmentini o'qib, localStorage'ga yozadi va manzilni tozalaydi", () => {
    window.history.replaceState(
      null,
      "",
      "/console#api_key=vb_live_abc123&token=eyJhbGciOiJIUzI1NiJ9.abc.def"
    );

    const key = consumeHashApiKey();

    expect(key).toBe("vb_live_abc123");
    expect(getApiKey()).toBe("vb_live_abc123");
    expect(getToken()).toBe("eyJhbGciOiJIUzI1NiJ9.abc.def");
    expect(window.location.hash).toBe("");
    expect(window.location.pathname).toBe("/console");
  });

  it("faqat token bo'lsa ham o'qiydi (kalit yo'q)", () => {
    window.history.replaceState(null, "", "/console#token=mock.jwt.tk");

    const key = consumeHashApiKey();

    expect(key).toBeNull();
    expect(getToken()).toBe("mock.jwt.tk");
    expect(window.location.hash).toBe("");
  });

  it("fragment bo'lmasa hech narsa qilmaydi", () => {
    window.history.replaceState(null, "", "/login");
    expect(consumeHashApiKey()).toBeNull();
  });
});

describe("status 90% yashil", () => {
  it("90 va undan yuqori — ok, 89 — yo'q", () => {
    expect(uptimeBarClass(90)).toContain("ok");
    expect(uptimeBarClass(99.2)).toContain("ok");
    expect(uptimeBarClass(100)).toContain("ok");
    expect(uptimeBarClass(89)).not.toContain("ok");
    expect(statusLooksOk("degraded", 91)).toBe(true);
    expect(statusLooksOk("operational", 100)).toBe(true);
    expect(statusLooksOk("degraded", 80)).toBe(false);
  });
});
