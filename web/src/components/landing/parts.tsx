
import { motion, type Variants } from "framer-motion";
import type { ReactNode } from "react";

export const K = ({ children }: { children: ReactNode }) => <span className="tok-k">{children}</span>;
export const S = ({ children }: { children: ReactNode }) => <span className="tok-s">{children}</span>;
export const N = ({ children }: { children: ReactNode }) => <span className="tok-n">{children}</span>;
export const F = ({ children }: { children: ReactNode }) => <span className="tok-f">{children}</span>;
export const C = ({ children }: { children: ReactNode }) => <span className="tok-c">{children}</span>;
export const B = ({ children }: { children: ReactNode }) => <span className="tok-b">{children}</span>;

export const fadeUp: Variants = {
  hidden: { y: 14 },
  show: { y: 0, transition: { duration: 0.5, ease: "easeOut" } },
};

export const stagger: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.09, delayChildren: 0.05 } },
};

export const scrollIn = {
  initial: { y: 16 },
  whileInView: { y: 0 },
  viewport: { once: true, margin: "-70px" },
  transition: { duration: 0.55, ease: "easeOut" as const },
};

export function Section({
  no,
  kicker,
  title,
  id,
  children,
}: {
  no: string;
  kicker: string;
  title: string;
  id?: string;
  children: ReactNode;
}) {
  return (
    <motion.section {...scrollIn} id={id} className="scroll-mt-16 border-t border-line">
      <div className="mx-auto max-w-[2000px] px-4 py-16 sm:px-6">
        <div className="micro mb-6 text-dim">
          <span className="text-accent2">{no}</span> — {kicker}
        </div>
        <h2 className="mb-10 text-2xl font-normal tracking-tight text-fg sm:text-3xl">{title}</h2>
        {children}
      </div>
    </motion.section>
  );
}
