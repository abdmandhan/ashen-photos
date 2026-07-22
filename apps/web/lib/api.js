const BASE = process.env.NEXT_PUBLIC_API_URL || "http://nuc.test:8080";

function token() {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("token");
}

async function req(path, { method = "GET", body, authed = true } = {}) {
  const headers = { "Content-Type": "application/json" };
  if (authed && token()) headers.Authorization = `Bearer ${token()}`;
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(data?.error || `HTTP ${res.status}`);
  return data;
}

export const api = {
  login: (email, password) =>
    req("/auth/login", { method: "POST", body: { email, password }, authed: false }),
  register: (email, password) =>
    req("/auth/register", { method: "POST", body: { email, password }, authed: false }),
  stats: () => req("/stats"),
  assets: (limit = 120) => req(`/assets?limit=${limit}`),
  devices: () => req("/devices"),
};

export function setToken(t) {
  localStorage.setItem("token", t);
}
export function clearToken() {
  localStorage.removeItem("token");
}
export function hasToken() {
  return !!token();
}
