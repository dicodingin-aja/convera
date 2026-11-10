import React, { useEffect, useRef, useState } from 'react';
import { ensureWorkspace, getBankAccounts, getMyProofs, getPlans, getSubscription, uploadProof } from './services/backend.js';
import { appConfig } from './services/config.js';
import { rupiah } from './plans.js';

// Modal bayar (mode backend), dibuat singkat: harga + isi paket, rekening tujuan, unggah bukti, kirim.
// Setelah terkirim tampil halaman "Pembayaran sedang diproses" + kontak CS. Langganan aktif setelah admin memverifikasi.
// Harga per slot kampanye: total = harga paket x jumlah kampanye x bulan (1 bulan = 30 hari).
// mode 'slots' = tambah slot di tengah langganan yang berjalan (selalu 30 hari, langganan utama tidak berubah).

const MAX_BYTES = 5 * 1024 * 1024;
const OK_EXT = /\.(jpe?g|png|pdf)$/i;
const box = { display: 'flex', flexDirection: 'column', gap: 12, padding: 16, borderRadius: 14, background: 'var(--surface2)' };
const mono = { fontFamily: 'ui-monospace, Menlo, Consolas, monospace' };
const STATUS_ID = { rejected: 'Ditolak', approved: 'Diterima', pending: 'Menunggu verifikasi' };

// "0812-3456-7890" / "+62 812..." -> "62812..." untuk tautan wa.me; tampilan tetap seperti diisi.
const waDigits = (p) => { const d = String(p).replace(/\D/g, ''); return d.startsWith('0') ? '62' + d.slice(1) : d; };
const featureText = (f) => (typeof f === 'string' ? f : f?.label || f?.name || '');

function Processing({ onClose }) {
  const cs = appConfig.csPhone;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 14, textAlign: 'center', padding: '12px 0 4px' }}>
      <span style={{ width: 64, height: 64, borderRadius: '50%', background: 'var(--accentSoft)', color: 'var(--accentText)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></svg>
      </span>
      <h2 id="pay-title" style={{ margin: 0, fontSize: 22, fontWeight: 600 }}>Pembayaran sedang diproses</h2>
      <p style={{ margin: 0, maxWidth: 420, color: 'var(--muted)', fontSize: 14, lineHeight: 1.55 }}>
        Bukti transfer sudah masuk. Kami cek dulu ke rekening, biasanya kurang dari 1×24 jam. Kamu dapat notifikasi begitu paket aktif.
      </p>
      {cs ? (
        <div style={{ ...box, alignSelf: 'stretch', alignItems: 'center', gap: 8 }}>
          <span style={{ fontSize: 13, color: 'var(--muted)' }}>Butuh bantuan atau pembayaran belum aktif setelah 1×24 jam? Hubungi CS kami</span>
          <b style={{ fontSize: 18, ...mono }}>{cs}</b>
          <a href={'https://wa.me/' + waDigits(cs)} target="_blank" rel="noreferrer" style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', height: 40, padding: '0 16px', borderRadius: 10, background: 'var(--surface)', color: 'var(--text)', fontSize: 14, fontWeight: 500, textDecoration: 'none' }}>Chat via WhatsApp</a>
        </div>
      ) : null}
      <button type="button" onClick={onClose} style={{ alignSelf: 'stretch', height: 46, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 600, cursor: 'pointer' }}>Kembali ke dashboard</button>
    </div>
  );
}

const MAX_SLOTS = 50;

export default function PaymentModal({ themeStyle, workspace, onClose, onDone, mode: startMode = 'subscription' }) {
  const [slug, setSlug] = useState(null);
  const [plans, setPlans] = useState([]);
  const [banks, setBanks] = useState([]);
  const [proofs, setProofs] = useState([]);
  const [sub, setSub] = useState(null);
  const [loadError, setLoadError] = useState('');
  const [f, setF] = useState({ plan: '', months: 1, bankAccount: '', slots: 1, purchase: startMode });
  const [file, setFile] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [copied, setCopied] = useState('');
  const fileRef = useRef(null);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const city = String(workspace.meta || '').split('·')[1]?.trim();
        const s = workspace.slug || (await ensureWorkspace(workspace.name, { city }));
        const [p, b, st, pr] = await Promise.all([getPlans(), getBankAccounts(s), getSubscription(s), getMyProofs(s)]);
        if (!alive) return;
        setSlug(s); setPlans(p); setBanks(b); setSub(st); setProofs(pr);
        const running = st && (st.status === 'active' || st.status === 'trial');
        // Perpanjang: mulai dari jumlah kampanye yang sedang dilacak. Tambah slot hanya untuk langganan berjalan.
        setF((x) => ({ ...x, plan: p[0]?.id || '', bankAccount: b[0]?.id || '', purchase: running ? x.purchase : 'subscription',
          slots: x.purchase === 'slots' && running ? 1 : Math.min(MAX_SLOTS, Math.max(1, st?.ad_slots_used || 1)) }));
      } catch (e) { if (alive) setLoadError(e.message || 'Gagal memuat'); }
    })();
    return () => { alive = false; };
  }, [workspace.name]);

  const plan = plans.find((p) => p.id === f.plan);
  const isSlots = f.purchase === 'slots';
  const months = isSlots ? 1 : f.months;
  const billed = !plan ? 0 : (months === 12 && plan.price12 ? plan.price12 : plan.price * months) * f.slots;
  const running = sub && (sub.status === 'active' || sub.status === 'trial');
  const setSlots = (n) => setF((x) => ({ ...x, slots: Math.min(MAX_SLOTS, Math.max(1, n)) }));
  const features = (plan?.features || []).map(featureText).filter(Boolean);
  const pendingFull = proofs.filter((p) => p.status === 'pending').length >= 3;
  const ready = !!file && !!plan && !!f.bankAccount && !pendingFull;

  const chooseFile = (e) => {
    const fl = e.target.files?.[0];
    setError('');
    if (!fl) { setFile(null); return; }
    if (!OK_EXT.test(fl.name)) { setError('Berkas harus JPG, PNG, atau PDF.'); e.target.value = ''; setFile(null); return; }
    if (fl.size > MAX_BYTES) { setError('Ukuran berkas maksimal 5 MB.'); e.target.value = ''; setFile(null); return; }
    setFile(fl);
  };

  const copy = (key, text) => {
    try { navigator.clipboard.writeText(text); } catch { /* abaikan */ }
    setCopied(key); setTimeout(() => setCopied(''), 1400);
  };

  const submit = async (e) => {
    e.preventDefault();
    if (!ready) return;
    const fd = new FormData();
    fd.append('plan', f.plan); fd.append('months', String(months)); fd.append('amount', String(billed));
    fd.append('ad_slots', String(f.slots)); fd.append('purchase', f.purchase);
    fd.append('bank_account_id', f.bankAccount);
    fd.append('file', file); // file terakhir agar isian teks sudah terbaca server
    setBusy(true); setError('');
    try { await uploadProof(slug, fd); setSent(true); onDone?.(); } catch (err) { setError(err.message || 'Gagal mengunggah'); } finally { setBusy(false); }
  };

  const loading = !slug && !loadError;
  const chip = (on) => ({ height: 38, padding: '0 14px', border: 'none', borderRadius: 10, cursor: 'pointer', fontFamily: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', boxShadow: on ? 'inset 0 0 0 2px var(--accent)' : 'none' });

  return (
    <div role="presentation" onMouseDown={sent ? undefined : onClose} style={{ ...themeStyle, position: 'fixed', inset: 0, zIndex: 90, background: 'rgba(0,0,0,.35)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
      <section role="dialog" aria-modal="true" aria-labelledby="pay-title" onMouseDown={(e) => e.stopPropagation()} style={{ ...themeStyle, width: 'min(520px, 100%)', maxHeight: '100%', overflowY: 'auto', background: 'var(--surface)', color: 'var(--text)', borderRadius: 18, padding: 24, display: 'flex', flexDirection: 'column', gap: 16, boxSizing: 'border-box' }}>
        {sent ? <Processing onClose={onClose} /> : (
          <>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
              <div>
                <h2 id="pay-title" style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>{isSlots ? 'Tambah slot kampanye' : 'Bayar langganan'}</h2>
                <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 14 }}>{workspace.name}</p>
              </div>
              <button type="button" onClick={onClose} aria-label="Tutup" style={{ border: 0, background: 'var(--surface2)', color: 'var(--text)', borderRadius: 8, width: 34, height: 34, cursor: 'pointer', fontSize: 20, lineHeight: 1 }}>×</button>
            </div>

            {loadError ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 14 }}>{loadError}</div> : loading ? <div style={{ color: 'var(--muted)' }}>Memuat…</div> : plans.length === 0 ? (
              <div style={{ color: 'var(--muted)' }}>Belum ada paket yang tersedia.</div>
            ) : (
              <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                {sub?.has_pending_proof ? <div style={{ ...box, fontSize: 13 }}>Masih ada bukti transfer yang sedang dicek. Kamu tetap bisa kirim bukti lain (maksimal 3 sekaligus).</div> : null}

                {running ? (
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button type="button" onClick={() => setF((x) => ({ ...x, purchase: 'subscription' }))} style={chip(!isSlots)}>Perpanjang langganan</button>
                    <button type="button" onClick={() => setF((x) => ({ ...x, purchase: 'slots', slots: 1 }))} style={chip(isSlots)}>Tambah slot kampanye</button>
                  </div>
                ) : null}

                {/* 1. Harga + isi paket */}
                <div style={box}>
                  {plans.length > 1 ? (
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      {plans.map((p) => <button key={p.id} type="button" onClick={() => setF((x) => ({ ...x, plan: p.id }))} style={chip(f.plan === p.id)}>{p.name}</button>)}
                    </div>
                  ) : null}
                  <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
                    <span style={{ fontSize: 15, fontWeight: 600 }}>{isSlots ? 'Slot kampanye tambahan' : 'Paket ' + (plan?.name || '')}</span>
                    <span><b style={{ fontSize: 26, letterSpacing: '-0.02em' }}>{rupiah(billed)}</b> <span style={{ fontSize: 13, color: 'var(--muted)' }}>/ {months * 30} hari</span></span>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
                    <span style={{ fontSize: 14 }}>Jumlah kampanye iklan<div style={{ fontSize: 12, color: 'var(--muted)' }}>{rupiah(plan?.price || 0)} per kampanye per 30 hari</div></span>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                      <button type="button" aria-label="Kurangi kampanye" onClick={() => setSlots(f.slots - 1)} disabled={f.slots <= 1} style={{ ...chip(false), width: 38, padding: 0 }}>−</button>
                      <input id="pay-slots" aria-label="Jumlah kampanye" inputMode="numeric" value={f.slots} onChange={(e) => setSlots(Number(e.target.value.replace(/\D/g, '')) || 1)} style={{ width: 52, height: 38, textAlign: 'center', border: 'none', borderRadius: 10, background: 'var(--surface)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 15, fontWeight: 600 }} />
                      <button type="button" aria-label="Tambah kampanye" onClick={() => setSlots(f.slots + 1)} disabled={f.slots >= MAX_SLOTS} style={{ ...chip(false), width: 38, padding: 0 }}>+</button>
                    </span>
                  </div>
                  {isSlots ? (
                    <span style={{ fontSize: 12, color: 'var(--muted)' }}>Slot tambahan aktif 30 hari sejak pembayaran disetujui.{sub?.ad_slots ? ` Sekarang ${sub.ad_slots_used || 0} dari ${sub.ad_slots} slot terpakai.` : ''}</span>
                  ) : (
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      {[1, 3, 12].map((m) => <button key={m} type="button" onClick={() => setF((x) => ({ ...x, months: m }))} style={chip(f.months === m)}>{m} bulan · {m * 30} hari</button>)}
                    </div>
                  )}
                  {features.length ? (
                    <ul style={{ margin: 0, padding: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 6, fontSize: 14 }}>
                      {features.map((t) => <li key={t} style={{ display: 'flex', gap: 8, alignItems: 'center' }}><span style={{ color: 'var(--accentText)', fontWeight: 700 }}>✓</span>{t}</li>)}
                    </ul>
                  ) : null}
                </div>

                {/* 2. Rekening tujuan */}
                <div style={box}>
                  <b style={{ fontSize: 14 }}>Transfer tepat {rupiah(billed)} ke rekening</b>
                  {banks.length === 0 ? <span style={{ color: 'var(--muted)', fontSize: 14 }}>Rekening tujuan belum tersedia. Hubungi CS Convera.</span> : banks.map((b) => (
                    <div key={b.id} onClick={() => setF((x) => ({ ...x, bankAccount: b.id }))} style={{ display: 'flex', gap: 10, alignItems: 'center', justifyContent: 'space-between', padding: 12, borderRadius: 10, background: 'var(--surface)', cursor: banks.length > 1 ? 'pointer' : 'default', boxShadow: banks.length > 1 && f.bankAccount === b.id ? 'inset 0 0 0 2px var(--accent)' : 'none' }}>
                      <span style={{ fontSize: 14 }}><b>{b.bank}</b> · <span style={mono}>{b.number}</span><div style={{ color: 'var(--muted)', fontSize: 12 }}>a.n. {b.holder}</div></span>
                      <button type="button" onClick={(e) => { e.stopPropagation(); copy(b.id, String(b.number)); }} style={{ height: 32, padding: '0 12px', border: 'none', borderRadius: 8, background: 'var(--surface2)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 12, fontWeight: 500, cursor: 'pointer' }}>{copied === b.id ? 'Tersalin' : 'Salin'}</button>
                    </div>
                  ))}
                </div>

                {/* 3. Unggah bukti */}
                <div style={box}>
                  <b style={{ fontSize: 14 }}>Unggah bukti transfer</b>
                  <input ref={fileRef} type="file" accept=".jpg,.jpeg,.png,.pdf,image/jpeg,image/png,application/pdf" onChange={chooseFile} style={{ display: 'none' }} />
                  <button type="button" onClick={() => fileRef.current?.click()} style={{ minHeight: 52, padding: '10px 14px', border: '1.5px dashed var(--borderStrong)', borderRadius: 10, background: 'var(--surface)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, cursor: 'pointer', textAlign: 'center' }}>
                    {file ? <span><b>{file.name}</b> <span style={{ color: 'var(--muted)' }}>· ganti berkas</span></span> : <span>Pilih berkas <span style={{ color: 'var(--muted)' }}>(JPG, PNG, PDF · maks. 5 MB)</span></span>}
                  </button>
                </div>

                {error ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{error}</div> : null}
                {pendingFull ? <div style={{ color: 'var(--muted)', fontSize: 13 }}>Sudah ada 3 bukti yang menunggu verifikasi. Tunggu hingga diperiksa.</div> : null}
                <button type="submit" disabled={!ready || busy} style={{ height: 48, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 15, fontWeight: 600, cursor: ready && !busy ? 'pointer' : 'not-allowed', opacity: ready && !busy ? 1 : 0.45 }}>{busy ? 'Mengirim…' : 'Kirim'}</button>

                {proofs.length ? (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <b style={{ fontSize: 13, color: 'var(--muted)' }}>Riwayat bukti transfer</b>
                    {proofs.slice(0, 3).map((p) => (
                      <div key={p.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 13, padding: '10px 12px', borderRadius: 10, background: 'var(--surface2)' }}>
                        <span>{new Date(p.created_at).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })} · {p.purchase === 'slots' ? 'Tambah slot' : p.plan_name} · {p.ad_slots || 1} kampanye · {p.months * 30} hari · {rupiah(p.billed)}</span>
                        <span style={{ color: p.status === 'rejected' ? 'var(--danger)' : 'var(--muted)', textAlign: 'right' }}>{STATUS_ID[p.status]}{p.reject_reason ? ': ' + p.reject_reason : ''}</span>
                      </div>
                    ))}
                  </div>
                ) : null}
              </form>
            )}
          </>
        )}
      </section>
    </div>
  );
}
