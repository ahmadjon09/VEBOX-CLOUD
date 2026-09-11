
export interface TierInfo {
  id: "free";
  price: string;
  storage: string;
  traffic: string;
  filesPerMonth: string;
  ratePerMin: string;
  hdWidth: string;
  fileLimit: string;
  analytics: boolean;
  resize: boolean;
}

export const TIERS: TierInfo[] = [
  {
    id: "free",
    price: "0$",
    storage: "∞",
    traffic: "∞",
    filesPerMonth: "∞",
    ratePerMin: "120",
    hdWidth: "4096 px",
    fileLimit: "50 MB",
    analytics: true,
    resize: true,
  },
];
