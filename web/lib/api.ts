// Thin client for the Go API. The token is a per-browser convenience kept in
// localStorage; every call degrades gracefully when storage is unavailable.

import { getLang } from "./i18n";

const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";

export function getToken(): string {
  try {
    return localStorage.getItem("gcplab.token") ?? "";
  } catch {
    return "";
  }
}

export function setToken(t: string) {
  try {
    if (t) localStorage.setItem("gcplab.token", t);
    else localStorage.removeItem("gcplab.token");
  } catch {
    /* storage unavailable: session-only login */
  }
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function api<T = any>(path: string, opts: { method?: string; body?: unknown } = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json", "X-Lang": getLang() };
  const t = getToken();
  if (t) headers.Authorization = `Bearer ${t}`;
  const res = await fetch(BASE + path, {
    method: opts.method ?? (opts.body !== undefined ? "POST" : "GET"),
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  const text = await res.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!res.ok) {
    if (res.status === 401 && typeof window !== "undefined" && !path.startsWith("/api/auth")) {
      setToken("");
      window.location.href = "/";
    }
    throw new ApiError(res.status, (data && data.error) || res.statusText);
  }
  return data as T;
}

export function qs(name: string): string {
  if (typeof window === "undefined") return "";
  return new URLSearchParams(window.location.search).get(name) ?? "";
}
