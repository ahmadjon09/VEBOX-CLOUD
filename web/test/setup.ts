
import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { unload } from "swr";

afterEach(() => {
  cleanup();
  unload();
  window.localStorage.clear();
});

if (typeof window !== "undefined") {
  if (!window.matchMedia) {
    window.matchMedia = ((q: string) => ({
      matches: false,
      media: q,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia;
  }
  if (!window.ResizeObserver) {
    (window as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
  if (!("IntersectionObserver" in window)) {
    (window as any).IntersectionObserver = class {
      private cb: IntersectionObserverCallback;
      constructor(cb: IntersectionObserverCallback) {
        this.cb = cb;
      }
      observe(t: Element) {
        queueMicrotask(() =>
          this.cb(
            [
              {
                target: t,
                isIntersecting: true,
                intersectionRatio: 1,
                boundingClientRect: t.getBoundingClientRect(),
                intersectionRect: t.getBoundingClientRect(),
                rootBounds: null,
                time: 0,
              },
            ] as IntersectionObserverEntry[],
            this as unknown as IntersectionObserver
          )
        );
      }
      unobserve() {}
      disconnect() {}
      takeRecords() {
        return [];
      }
    };
  }
  if (!window.HTMLElement.prototype.scrollTo) {
    window.HTMLElement.prototype.scrollTo = () => {};
  }
  if (!window.scrollTo) {
    window.scrollTo = (() => {}) as typeof window.scrollTo;
  }
}
