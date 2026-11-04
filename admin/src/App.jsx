import React, { useEffect, useState } from 'react';
import { Ctx } from './ui.jsx';
import { Icon, Logo } from './Icon.jsx';
import { useStore } from './store.js';
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

export default function App() {
  const [session, setSession] = useState(getSession());
  const [s, update, reset] = useStore();
  const [page, setPage] = useState(() => { try { return sessionStorage.getItem('convera-admin:page') || 'ringkasan'; } catch { return 'ringkasan'; } });
  useEffect(() => { document.documentElement.dataset.theme = s.dark ? 'dark' : 'light'; }, [s.dark]);
  useEffect(() => { try { sessionStorage.setItem('convera-admin:page', page); } catch { /* abaikan */ } }, [page]);

  if (!session) return <Login onLogin={setSession} />;
  const pending = s.proofs.filter((p) => p.status === 'pending').length;
  const Page = (PAGES.find((p) => p[0] === page) || PAGES[0])[3];
  return (
    <Ctx.Provider value={{ s, update, go: setPage, reset }}>
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
            <div className="row nowrap"><span className="av ink round">A</span><span style={{ fontWeight: 500 }}>Admin</span></div>
            <div className="row nowrap">
              <button className="btn s" style={{ flex: 1 }} onClick={() => update((x) => ({ ...x, dark: !x.dark }))}><Icon name={s.dark ? 'sun' : 'moon'} size={16} />{s.dark ? 'Terang' : 'Gelap'}</button>
              <button className="btn s" style={{ flex: 1 }} onClick={() => { logout(); setSession(null); }}><Icon name="logout" size={16} />Keluar</button>
            </div>
          </div>
        </aside>
        <main className="main"><Page /></main>
      </div>
    </Ctx.Provider>
  );
}
