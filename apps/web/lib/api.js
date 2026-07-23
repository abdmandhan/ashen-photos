const BASE = process.env.NEXT_PUBLIC_API_URL || "https://8082.abdmandhan.com";

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
    req("/auth/login", {
      method: "POST",
      body: { email, password },
      authed: false,
    }),
  register: (email, password) =>
    req("/auth/register", {
      method: "POST",
      body: { email, password },
      authed: false,
    }),
  stats: () => req("/stats"),
  facets: () => req("/search/facets"),
  assets: (params = {}) => {
    const q = new URLSearchParams({ limit: 120, ...params }).toString();
    return req(`/assets?${q}`);
  },
  devices: () => req("/devices"),
  albums: () => req("/albums"),
  createAlbum: (name) => req("/albums", { method: "POST", body: { name } }),
  deleteAlbum: (id) => req(`/albums/${id}`, { method: "DELETE" }),
  albumAssets: (id) => req(`/albums/${id}/assets`),
  addToAlbum: (albumId, assetId) =>
    req(`/albums/${albumId}/assets`, {
      method: "POST",
      body: { asset_id: assetId },
    }),
  removeFromAlbum: (albumId, assetId) =>
    req(`/albums/${albumId}/assets/${assetId}`, { method: "DELETE" }),
  favorite: (assetId, favorite) =>
    req(`/assets/${assetId}/favorite`, { method: "PUT", body: { favorite } }),
  duplicates: () => req("/duplicates"),
  resolveDuplicate: (assetId, action) =>
    req(`/assets/${assetId}/resolve-duplicate`, {
      method: "POST",
      body: { action },
    }),
  replicationStatus: () => req("/replication/status"),
  redrive: () => req("/replication/redrive", { method: "POST" }),
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
