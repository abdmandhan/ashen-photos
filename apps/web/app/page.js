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

const FILTERS = [
  { key: "all", label: "All", params: {} },
  { key: "photo", label: "Photos", params: { media_type: "photo" } },
  { key: "video", label: "Videos", params: { media_type: "video" } },
  { key: "favorite", label: "Favorites", params: { favorite: "true" } },
];

function Dashboard({ onLogout }) {
  const [stats, setStats] = useState(null);
  const [facets, setFacets] = useState(null);
  const [assets, setAssets] = useState([]);
  const [devices, setDevices] = useState([]);
  const [albums, setAlbums] = useState([]);
  const [dups, setDups] = useState([]);
  const [repl, setRepl] = useState(null);
  const [filter, setFilter] = useState("all");
  const [sort, setSort] = useState("newest");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(null);
  const [preview, setPreview] = useState(null);      // asset shown in lightbox
  const [albumView, setAlbumView] = useState(null);  // { album, assets }
  const [addMenu, setAddMenu] = useState(null);       // asset id whose add-to-album menu is open

  const PAGE = 100;

  function buildParams(off) {
    const f = FILTERS.find((x) => x.key === filter) || FILTERS[0];
    const p = { limit: PAGE, offset: off, sort, ...f.params };
    if (fromDate) p.from = `${fromDate}T00:00:00Z`;
    if (toDate) p.to = `${toDate}T23:59:59Z`;
    return p;
  }

  // Load the first page (on filter/sort/date change). Later pages append.
  async function loadTimeline(nextFilter = filter) {
    const f = FILTERS.find((x) => x.key === nextFilter) || FILTERS[0];
    const p = { limit: PAGE, offset: 0, sort, ...f.params };
    if (fromDate) p.from = `${fromDate}T00:00:00Z`;
    if (toDate) p.to = `${toDate}T23:59:59Z`;
    const a = await api.assets(p);
    const list = a.assets || [];
    setAssets(list);
    setOffset(list.length);
    setHasMore(list.length >= PAGE);
  }

  async function loadMore() {
    if (!hasMore || loadingMore) return;
    setLoadingMore(true);
    try {
      const a = await api.assets(buildParams(offset));
      const list = a.assets || [];
      setAssets((prev) => {
        const seen = new Set(prev.map((x) => x.id));
        return [...prev, ...list.filter((x) => !seen.has(x.id))];
      });
      setOffset((o) => o + list.length);
      setHasMore(list.length >= PAGE);
    } catch (e) { setError(e.message); }
    finally { setLoadingMore(false); }
  }

  async function refreshMeta() {
    const [s, fc, d, al, du, rp] = await Promise.all([
      api.stats(), api.facets(), api.devices(), api.albums(),
      api.duplicates(), api.replicationStatus(),
    ]);
    setStats(s); setFacets(fc); setDevices(d || []); setAlbums(al.albums || []);
    setDups(du.groups || []); setRepl(rp);
  }

  useEffect(() => {
    (async () => {
      try {
        await refreshMeta();
        await loadTimeline("all");
      } catch (e) {
        setError(e.message);
      }
    })();
  }, []);

  // Reload the first page whenever sort or the date range changes.
  useEffect(() => {
    loadTimeline().catch((e) => setError(e.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sort, fromDate, toDate]);

  async function resolveDup(assetId, action) {
    try { await api.resolveDuplicate(assetId, action); await refreshMeta(); await loadTimeline(); }
    catch (e) { setError(e.message); }
  }

  async function runRedrive() {
    try { const r = await api.redrive(); await refreshMeta(); alert(`Queued ${r.queued} for replication`); }
    catch (e) { setError(e.message); }
  }

  async function selectFilter(key) {
    setFilter(key);
    try { await loadTimeline(key); } catch (e) { setError(e.message); }
  }

  async function toggleFavorite(asset, e) {
    e.preventDefault();
    try {
      await api.favorite(asset.id, !asset.favorite);
      setAssets((prev) => prev.map((x) => (x.id === asset.id ? { ...x, favorite: !x.favorite } : x))
        .filter((x) => filter !== "favorite" || x.favorite));
      const fc = await api.facets(); setFacets(fc);
    } catch (e2) { setError(e2.message); }
  }

  async function newAlbum() {
    const name = prompt("Album name");
    if (!name) return;
    try { await api.createAlbum(name); const al = await api.albums(); setAlbums(al.albums || []); }
    catch (e) { setError(e.message); }
  }

  async function openAlbum(al) {
    try { const r = await api.albumAssets(al.id); setAlbumView({ album: al, assets: r.assets || [] }); }
    catch (e) { setError(e.message); }
  }

  async function deleteAlbum() {
    if (!albumView || !confirm(`Delete album “${albumView.album.name}”? Your photos stay.`)) return;
    try { await api.deleteAlbum(albumView.album.id); setAlbumView(null); await refreshMeta(); }
    catch (e) { setError(e.message); }
  }

  async function removeFromAlbum(assetId) {
    try {
      await api.removeFromAlbum(albumView.album.id, assetId);
      const r = await api.albumAssets(albumView.album.id);
      setAlbumView({ ...albumView, assets: r.assets || [] });
      await refreshMeta();
    } catch (e) { setError(e.message); }
  }

  async function addToAlbum(albumId, assetId) {
    try { await api.addToAlbum(albumId, assetId); setAddMenu(null); await refreshMeta(); }
    catch (e) { setError(e.message); }
  }

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
          <div className="stat"><div className="value">{facets?.favorite_count ?? "—"}</div><div className="label">Favorites</div></div>
          <div className="stat"><div className="value">{stats ? fmtBytes(stats.total_bytes) : "—"}</div><div className="label">Storage used</div></div>
          <div className="stat"><div className="value">{devices.length}</div><div className="label">Devices</div></div>
        </div>

        <div className="section-header">
          <div className="section-title">Albums</div>
          <button className="link" onClick={newAlbum}>+ New album</button>
        </div>
        <div className="albums">
          {albums.length === 0 && <div className="muted">No albums yet.</div>}
          {albums.map((al) => (
            <div className="album album-clickable" key={al.id} onClick={() => openAlbum(al)}>
              <div className="album-cover">
                {al.cover_url ? <img src={al.cover_url} alt="" /> : <div className="placeholder">empty</div>}
              </div>
              <div className="album-name">{al.name}</div>
              <div className="meta">{al.asset_count} item{al.asset_count === 1 ? "" : "s"}</div>
            </div>
          ))}
        </div>

        {dups.length > 0 && (
          <>
            <div className="section-title">Possible duplicates ({dups.length} group{dups.length === 1 ? "" : "s"})</div>
            {dups.map((g) => (
              <div className="dup-group" key={g.group_id}>
                {g.assets.map((a) => (
                  <div className="dup-item" key={a.id}>
                    {a.thumb_url ? <img src={a.thumb_url} alt="" /> : <div className="placeholder">{a.media_type}</div>}
                    <div className="dup-actions">
                      <button className="chip" onClick={() => resolveDup(a.id, "keep")}>Keep</button>
                      <button className="chip chip-danger" onClick={() => resolveDup(a.id, "delete")}>Delete</button>
                    </div>
                    <div className="meta">{fmtBytes(a.byte_size)}</div>
                  </div>
                ))}
              </div>
            ))}
          </>
        )}

        <div className="section-header">
          <div className="section-title">Devices</div>
          {repl && (
            <div className="repl">
              <span className="muted">Replication:</span> {repl.replicated} ok
              {repl.failed > 0 && <span className="repl-fail"> · {repl.failed} failed</span>}
              {repl.unreplicated > 0 && <span className="muted"> · {repl.unreplicated} pending</span>}
              {(repl.failed > 0 || repl.unreplicated > 0) && (
                <button className="link" onClick={runRedrive}>Redrive</button>
              )}
            </div>
          )}
        </div>
        <div className="devices">
          {devices.length === 0 && <div className="muted">No devices yet.</div>}
          {devices.map((d) => (
            <div className="device" key={d.id}>
              <div>{d.name}</div>
              <div className="meta">
                {d.uploaded_count} item{d.uploaded_count === 1 ? "" : "s"}
                {d.last_seen_at ? ` · seen ${new Date(d.last_seen_at).toLocaleDateString()}` : " · never seen"}
              </div>
            </div>
          ))}
        </div>

        <div className="section-title">Timeline</div>
        <div className="filters">
          {FILTERS.map((f) => (
            <button
              key={f.key}
              className={`chip ${filter === f.key ? "chip-active" : ""}`}
              onClick={() => selectFilter(f.key)}
            >
              {f.label}
            </button>
          ))}
        </div>
        <div className="toolbar">
          <label className="tool">
            Sort
            <select value={sort} onChange={(e) => setSort(e.target.value)}>
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
            </select>
          </label>
          <label className="tool">From
            <input type="date" value={fromDate} onChange={(e) => setFromDate(e.target.value)} />
          </label>
          <label className="tool">To
            <input type="date" value={toDate} onChange={(e) => setToDate(e.target.value)} />
          </label>
          {(fromDate || toDate) && (
            <button className="link" onClick={() => { setFromDate(""); setToDate(""); }}>Clear dates</button>
          )}
        </div>
        {assets.length === 0 && <div className="muted">Nothing here.</div>}
        <div className="grid">
          {assets.map((a) => (
            <div className="tile" key={a.id} onClick={() => setPreview(a)} title="Preview">
              {a.thumb_url ? (
                <img src={a.thumb_url} alt="" loading="lazy" />
              ) : (
                <div className="placeholder">{a.media_type}</div>
              )}
              {a.media_type === "video" && <span className="badge">▶</span>}
              <button
                className={`heart ${a.favorite ? "heart-on" : ""}`}
                onClick={(e) => toggleFavorite(a, e)}
                title={a.favorite ? "Unfavorite" : "Favorite"}
              >
                {a.favorite ? "♥" : "♡"}
              </button>
              {albums.length > 0 && (
                <button
                  className="add-btn"
                  onClick={(e) => { e.stopPropagation(); setAddMenu(addMenu === a.id ? null : a.id); }}
                  title="Add to album"
                >
                  +
                </button>
              )}
              {addMenu === a.id && (
                <div className="add-menu" onClick={(e) => e.stopPropagation()}>
                  {albums.map((al) => (
                    <button key={al.id} onClick={() => addToAlbum(al.id, a.id)}>{al.name}</button>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
        {hasMore && assets.length > 0 && (
          <div style={{ textAlign: "center", marginTop: 16 }}>
            <button className="btn" style={{ width: "auto", padding: "10px 24px" }}
              onClick={loadMore} disabled={loadingMore}>
              {loadingMore ? "Loading…" : "Load more"}
            </button>
          </div>
        )}
      </div>

      {preview && (
        <Lightbox asset={preview} onClose={() => setPreview(null)} />
      )}

      {albumView && (
        <AlbumModal
          view={albumView}
          onClose={() => setAlbumView(null)}
          onDelete={deleteAlbum}
          onRemove={removeFromAlbum}
          onPreview={setPreview}
        />
      )}
    </>
  );
}

function Lightbox({ asset, onClose }) {
  const isVideo = asset.media_type === "video";
  return (
    <div className="lightbox" onClick={onClose}>
      <button className="lightbox-close" onClick={onClose}>✕</button>
      <div className="lightbox-body" onClick={(e) => e.stopPropagation()}>
        {isVideo ? (
          <video src={asset.download_url} controls autoPlay className="lightbox-media" />
        ) : (
          <img src={asset.download_url} alt="" className="lightbox-media" />
        )}
        <a className="lightbox-dl" href={asset.download_url} target="_blank" rel="noreferrer">
          Download original
        </a>
      </div>
    </div>
  );
}

function AlbumModal({ view, onClose, onDelete, onRemove, onPreview }) {
  return (
    <div className="lightbox" onClick={onClose}>
      <div className="album-modal" onClick={(e) => e.stopPropagation()}>
        <div className="album-modal-head">
          <h2>{view.album.name}</h2>
          <div>
            <button className="chip chip-danger" onClick={onDelete}>Delete album</button>
            <button className="link" onClick={onClose}>Close</button>
          </div>
        </div>
        {view.assets.length === 0 ? (
          <div className="muted">No items. Add photos with the + button on a tile.</div>
        ) : (
          <div className="grid">
            {view.assets.map((a) => (
              <div className="tile" key={a.id} onClick={() => onPreview(a)}>
                {a.thumb_url ? <img src={a.thumb_url} alt="" /> : <div className="placeholder">{a.media_type}</div>}
                <button
                  className="add-btn remove-btn"
                  onClick={(e) => { e.stopPropagation(); onRemove(a.id); }}
                  title="Remove from album"
                >
                  −
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
