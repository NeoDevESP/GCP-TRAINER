// Thin client for the Go API.
//
// Session: on the same origin (the normal case: the Go server serves this
// web) the session is an HttpOnly cookie the server sets at sign-in; scripts
// never see the token, and every request carries X-CM-Client, which the
// server requires for changes made with the cookie (CSRF). Only a non-secret
// "signed in" mark is kept here. When the API is on another origin
// (NEXT_PUBLIC_API_BASE), cookies are not shared, so the token is kept and
// sent as Bearer; a token stored by an earlier version is also still sent.

import { getLang } from "./i18n";

const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";
const TOKEN = "gcplab.token"; // Bearer token (other-origin API, or older versions)
const MARK = "gcplab.signedIn"; // non-secret: a cookie session exists

function read(k: string) {
  try {
    return localStorage.getItem(k) ?? "";
  } catch {
    return "";
  }
}

/** getToken returns a Bearer token when one is kept (see above); usually "". */
export function getToken(): string {
  return read(TOKEN);
}

/** isSignedIn: there is a session (cookie mark or Bearer token). */
export function isSignedIn(): boolean {
  return !!read(MARK) || !!read(TOKEN);
}

/** setToken records a new session ("" signs out locally). */
export function setToken(t: string) {
  try {
    if (!t) {
      localStorage.removeItem(TOKEN);
      localStorage.removeItem(MARK);
    } else if (BASE) {
      localStorage.setItem(TOKEN, t);
    } else {
      localStorage.removeItem(TOKEN); // the cookie carries the session
      localStorage.setItem(MARK, "1");
    }
  } catch {
    /* storage unavailable: session-only login */
  }
}

/** keepBearer stores a token that did not come with a cookie (old links). */
export function keepBearer(t: string) {
  try {
    localStorage.setItem(TOKEN, t);
  } catch {
    /* storage unavailable */
  }
}

/** signOut ends the session on the server (cookie) and in this browser. */
export async function signOut() {
  await api("/api/auth/logout", { body: {} }).catch(() => {});
  setToken("");
}

export class ApiError extends Error {
  constructor(public status: number, message: string, public data?: any) {
    super(message);
  }
}

export async function api<T = any>(path: string, opts: { method?: string; body?: unknown; headers?: Record<string, string> } = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json", "X-Lang": getLang(), ...(opts.headers ?? {}) };
  if (!BASE) headers["X-CM-Client"] = "web";
  const t = getToken();
  if (t) headers.Authorization = `Bearer ${t}`;
  const res = await fetch(BASE + path, {
    credentials: "same-origin",
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
    throw new ApiError(res.status, (data && data.error) || res.statusText, data);
  }
  return data as T;
}

export function qs(name: string): string {
  if (typeof window === "undefined") return "";
  return new URLSearchParams(window.location.search).get(name) ?? "";
}
