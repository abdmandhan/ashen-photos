"use client";

import { useEffect, useState } from "react";
import { api, setToken, clearToken, hasToken } from "../lib/api";

export default function Home() {
  const [authed, setAuthed] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    setAuthed(hasToken());
    setReady(true);
  }, []);

  if (!ready) return null;
  return authed ? (
    <Dashboard onLogout={() => { clearToken(); setAuthed(false); }} />
  ) : (
    <Login onAuthed={() => setAuthed(true)} />
  );
}

function Login({ onAuthed }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isRegister, setIsRegister] = useState(false);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  async function submit() {
    setBusy(true); setError(null);
    try {
      const fn = isRegister ? api.register : api.login;
      const res = await fn(email, password);
      setToken(res.token);
      onAuthed();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <h1>Ashen Photos</h1>
      {error && <div className="error">{error}</div>}
      <input placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} />
      <input placeholder="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      <button className="btn" onClick={submit} disabled={busy || !email || password.length < 8}>
        {isRegister ? "Create account" : "Log in"}
      </button>
      <button className="link" onClick={() => setIsRegister(!isRegister)}>
        {isRegister ? "Have an account? Log in" : "New here? Create account"}
      </button>
    </div>
  );
}

function fmtBytes(n) {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(n) / Math.log(1024));
  return `${(n / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
}

function Dashboard({ onLogout }) {
  const [stats, setStats] = useState(null);
  const [assets, setAssets] = useState([]);
  const [devices, setDevices] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    (async () => {
      try {
        const [s, a, d] = await Promise.all([api.stats(), api.assets(), api.devices()]);
        setStats(s);
        setAssets(a.assets || []);
        setDevices(d || []);
      } catch (e) {
        setError(e.message);
      }
    })();
  }, []);

  return (
    <>
      <div className="header">
        <h1>Ashen Photos</h1>
        <button className="link" onClick={onLogout}>Log out</button>
      </div>
      <div className="container">
        {error && <div className="error">{error}</div>}

        <div className="stats">
          <div className="stat"><div className="value">{stats?.photo_count ?? "—"}</div><div className="label">Photos</div></div>
          <div className="stat"><div className="value">{stats?.video_count ?? "—"}</div><div className="label">Videos</div></div>
          <div className="stat"><div className="value">{stats ? fmtBytes(stats.total_bytes) : "—"}</div><div className="label">Storage used</div></div>
          <div className="stat"><div className="value">{devices.length}</div><div className="label">Devices</div></div>
        </div>

        <div className="section-title">Devices</div>
        <div className="devices">
          {devices.length === 0 && <div className="muted">No devices yet.</div>}
          {devices.map((d) => (
            <div className="device" key={d.id}>
              <div>{d.name}</div>
              <div className="meta">{d.platform} · added {new Date(d.created_at).toLocaleDateString()}</div>
            </div>
          ))}
        </div>

        <div className="section-title">Timeline</div>
        {assets.length === 0 && <div className="muted">No backed-up media yet.</div>}
        <div className="grid">
          {assets.map((a) => (
            <a className="tile" key={a.id} href={a.download_url} target="_blank" rel="noreferrer" title="Download original">
              {a.thumb_url ? (
                <img src={a.thumb_url} alt="" loading="lazy" />
              ) : (
                <div className="placeholder">{a.media_type}</div>
              )}
              {a.media_type === "video" && <span className="badge">▶</span>}
            </a>
          ))}
        </div>
      </div>
    </>
  );
}
