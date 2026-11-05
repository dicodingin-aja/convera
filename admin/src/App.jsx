import React, { useCallback, useEffect, useState } from 'react';
import { Ctx } from './ui.jsx';
import { Icon, Logo } from './Icon.jsx';
import { api, setUnauthorizedHandler } from './api.js';
import { getSession, logout } from './auth.js';
import Login from './Login.jsx';
import Ringkasan from './pages/Ringkasan.jsx';
import Verifikasi from './pages/Verifikasi.jsx';
import Customer from './pages/Customer.jsx';
import BagiHasil from './pages/BagiHasil.jsx';
import Paket from './pages/Paket.jsx';
import Integrasi from './pages/Integrasi.jsx';
import Developer from './pages/Developer.jsx';

const PAGES = [
  ['ringkasan', 'Ringkasan', 'dash', Ringkasan], ['verifikasi', 'Verifikasi Pembayaran', 'receipt', Verifikasi], ['customer', 'Customer', 'users', Customer],
  ['bagi', 'Bagi Hasil', 'pie', BagiHasil], ['paket', 'Paket & Harga', 'box', Paket], ['integrasi', 'Integrasi', 'plug', Integrasi], ['developer', 'Developer', 'code', Developer],
];

const read = (k, d) => { try { return localStorage.getItem(k) ?? d; } catch { return d; } };

export default function App() {
  const [me, setMe] = useState(undefined); // undefined = memeriksa sesi, null = belum login
  const [fatal, setFatal] = useState('');
  const [pending, setPending] = useState(0);
  const [dark, setDark] = useState(() => read('convera-admin:dark', '0') === '1');
  const [page, setPage] = useState(() => { try { return sessionStorage.getItem('convera-admin:page') || 'ringkasan'; } catch { return 'ringkasan'; } });

  useEffect(() => { document.documentElement.dataset.theme = dark ? 'dark' : 'light'; try { localStorage.setItem('convera-admin:dark', dark ? '1' : '0'); } catch { /* abaikan */ } }, [dark]);
  useEffect(() => { try { sessionStorage.setItem('convera-admin:page', page); } catch { /* abaikan */ } }, [page]);
  useEffect(() => { setUnauthorizedHandler(() => setMe(null)); }, []);
  useEffect(() => { getSession().then(setMe).catch((e) => { setFatal(e.message); setMe(null); }); }, []);

  const refreshPending = useCallback(() => {
    api.get('/payment-proofs?status=pending&per_page=1').then((r) => setPending(r.meta.total)).catch(() => {});
  }, []);
  useEffect(() => {
    if (!me) return undefined;
    refreshPending();
    const t = setInterval(refreshPending, 60000); // lonceng: cek bukti baru tiap menit
    return () => clearInterval(t);
  }, [me, refreshPending]);

  if (me === undefined) return <div style={{ padding: 40 }} className="muted">Memeriksa sesi…</div>;
  if (!me) return <Login onLogin={setMe} notice={fatal} />;

  const Page = (PAGES.find((p) => p[0] === page) || PAGES[0])[3];
  return (
    <Ctx.Provider value={{ me, pending, refreshPending, go: setPage }}>
      <div className="shell">
        <aside className="side">
          <div className="brand"><Logo /> Convera <span className="tag">ADMIN</span></div>
          <nav className="nav">
            {PAGES.map(([k, label, icon]) => (
              <button key={k} className={page === k ? 'on' : ''} onClick={() => setPage(k)}>
                <Icon name={icon} />{label}{k === 'verifikasi' && pending ? <span className="count">{pending}</span> : null}
              </button>
            ))}
          </nav>
          <div className="who">
            <div className="row nowrap"><span className="av ink round">{(me.name || me.username).slice(0, 1).toUpperCase()}</span><span><span style={{ fontWeight: 500 }}>{me.name || me.username}</span><div className="muted sm">{me.role}</div></span></div>
            <div className="row nowrap">
              <button className="btn s" style={{ flex: 1 }} onClick={() => setDark((d) => !d)}><Icon name={dark ? 'sun' : 'moon'} size={16} />{dark ? 'Terang' : 'Gelap'}</button>
              <button className="btn s" style={{ flex: 1 }} onClick={async () => { await logout(); setMe(null); }}><Icon name="logout" size={16} />Keluar</button>
            </div>
          </div>
        </aside>
        <main className="main"><Page key={page} /></main>
      </div>
    </Ctx.Provider>
  );
}
