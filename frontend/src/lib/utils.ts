import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import { getLang, translate } from "@/lib/i18n";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatTime(value?: string | null) {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "-";
  return d.toLocaleString(getLang() === "zh" ? "zh-CN" : "en-US");
}

export function formatNumber(value: number | null | undefined, digits = 2) {
  if (value === null || value === undefined) return "-";
  return Number(value).toFixed(digits).replace(/\.?0+$/, "");
}

export function sinceText(value?: string | null): string {
  if (!value) return "-";
  const t = new Date(value).getTime();
  if (Number.isNaN(t)) return "-";
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  const lang = getLang();
  if (s < 60) return translate(lang, "time.secondsAgo", { n: s });
  if (s < 3600) return translate(lang, "time.minutesAgo", { n: Math.floor(s / 60) });
  if (s < 86400) return translate(lang, "time.hoursAgo", { n: Math.floor(s / 3600) });
  return translate(lang, "time.daysAgo", { n: Math.floor(s / 86400) });
}

export function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
