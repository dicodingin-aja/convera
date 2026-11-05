import React, { useEffect, useState } from 'react';
import { ensureWorkspace, getBankAccounts, getMyProofs, getPlans, getSubscription, uploadProof } from './services/backend.js';
import { rupiah } from './plans.js';

// Modal bayar untuk mode backend: pilih paket + periode, transfer ke rekening Convera, lalu unggah bukti.
// Langganan aktif setelah admin memverifikasi bukti.

const MAX_BYTES = 5 * 1024 * 1024;
const OK_EXT = /\.(jpe?g|png|pdf)$/i;
const field = { width: '100%', height: 44, padding: '0 12px', border: 'none', borderRadius: 10, background: 'var(--surface)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };
const label = { display: 'flex', flexDirection: 'column', gap: 6, fontSize: 13, fontWeight: 500, minWidth: 0 };
const box = { display: 'flex', flexDirection: 'column', gap: 12, padding: 16, borderRadius: 14, background: 'var(--surface2)' };
const STATUS_ID = { rejected: 'Ditolak', approved: 'Diterima', pending: 'Menunggu verifikasi' };

export default function PaymentModal({ themeStyle, workspace, onClose, onDone }) {
  const [slug, setSlug] = useState(null);
  const [plans, setPlans] = useState([]);
  const [banks, setBanks] = useState([]);
  const [proofs, setProofs] = useState([]);
  const [sub, setSub] = useState(null);
  const [loadError, setLoadError] = useState('');
  const [f, setF] = useState({ plan: '', months: 1, bank: '', sender: '', ref: '', amount: '', bankAccount: '', message: '' });
  const [file, setFile] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const city = String(workspace.meta || '').split('·')[1]?.trim();
        const s = workspace.slug || (await ensureWorkspace(workspace.name, { city }));
        const [p, b, st, pr] = await Promise.all([getPlans(), getBankAccounts(s), getSubscription(s), getMyProofs(s)]);
        if (!alive) return;
        setSlug(s); setPlans(p); setBanks(b); setSub(st); setProofs(pr);
        setF((x) => ({ ...x, plan: p[0]?.id || '', bankAccount: b[0]?.id || '' }));
      } catch (e) { if (alive) setLoadError(e.message || 'Gagal memuat'); }
    })();
    return () => { alive = false; };
  }, [workspace.name]);

  const plan = plans.find((p) => p.id === f.plan);
  const billed = !plan ? 0 : f.months === 12 && plan.price12 ? plan.price12 : plan.price * f.months;
  const set = (k) => (e) => { setF((x) => ({ ...x, [k]: e.target.value })); setError(''); };
  const pickMonths = (m) => () => setF((x) => ({ ...x, months: m, amount: '' }));

  const chooseFile = (e) => {
    const fl = e.target.files?.[0];
    setError('');
    if (!fl) { setFile(null); return; }
    if (!OK_EXT.test(fl.name)) { setError('Berkas harus JPG, PNG, atau PDF.'); e.target.value = ''; setFile(null); return; }
    if (fl.size > MAX_BYTES) { setError('Ukuran berkas maksimal 5 MB.'); e.target.value = ''; setFile(null); return; }
    setFile(fl);
  };

  const submit = async (e) => {
    e.preventDefault();
    if (!file) { setError('Lampirkan bukti transfer.'); return; }
    if (!f.bank.trim() || !f.sender.trim()) { setError('Isi bank dan nama pemilik rekening pengirim.'); return; }
    const fd = new FormData();
    fd.append('plan', f.plan); fd.append('months', String(f.months)); fd.append('bank', f.bank.trim()); fd.append('sender', f.sender.trim());
    if (f.ref.trim()) fd.append('ref', f.ref.trim());
    fd.append('amount', String(f.amount.trim() ? f.amount.replace(/\D/g, '') : billed));
    if (f.bankAccount) fd.append('bank_account_id', f.bankAccount);
    if (f.message.trim()) fd.append('message', f.message.trim());
    fd.append('file', file); // file terakhir agar isian teks sudah terbaca server
    setBusy(true); setError('');
    try { await uploadProof(slug, fd); setSent(true); onDone?.(); } catch (err) { setError(err.message || 'Gagal mengunggah'); } finally { setBusy(false); }
  };

  const loading = !slug && !loadError;
  const pendingFull = proofs.filter((p) => p.status === 'pending').length >= 3;

  return (
    <div role="presentation" onMouseDown={onClose} style={{ ...themeStyle, position: 'fixed', inset: 0, zIndex: 90, background: 'rgba(0,0,0,.35)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
      <section role="dialog" aria-modal="true" aria-labelledby="pay-title" onMouseDown={(e) => e.stopPropagation()} style={{ ...themeStyle, width: 'min(640px, 100%)', maxHeight: '100%', overflowY: 'auto', background: 'var(--surface)', color: 'var(--text)', borderRadius: 18, padding: 24, display: 'flex', flexDirection: 'column', gap: 16, boxSizing: 'border-box' }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div>
            <h2 id="pay-title" style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>Bayar langganan</h2>
            <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 14 }}>{workspace.name}</p>
          </div>
          <button type="button" onClick={onClose} aria-label="Tutup" style={{ border: 0, background: 'var(--surface2)', color: 'var(--text)', borderRadius: 8, width: 34, height: 34, cursor: 'pointer', fontSize: 20, lineHeight: 1 }}>×</button>
        </div>

        {loadError ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 14 }}>{loadError}</div> : loading ? <div style={{ color: 'var(--muted)' }}>Memuat…</div> : sent ? (
          <div style={box}>
            <b style={{ fontSize: 16 }}>Bukti transfer terkirim</b>
            <span style={{ color: 'var(--muted)', fontSize: 14 }}>Tim Convera akan mencocokkannya dengan mutasi rekening. Langganan aktif setelah diverifikasi, Anda akan mendapat notifikasi.</span>
            <button type="button" onClick={onClose} style={{ height: 44, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 500, cursor: 'pointer' }}>Selesai</button>
          </div>
        ) : (
          <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {sub?.has_pending_proof ? <div style={{ ...box, background: 'var(--surface2)', fontSize: 13 }}>Ada bukti transfer yang sedang menunggu verifikasi. Anda tetap bisa mengirim bukti lain (maksimal 3 menunggu).</div> : null}
            {plans.length === 0 ? <div style={{ color: 'var(--muted)' }}>Belum ada paket yang tersedia.</div> : (
              <>
                <div style={box}>
                  <label style={label}>Paket
                    <select value={f.plan} onChange={set('plan')} style={field}>{plans.map((p) => <option key={p.id} value={p.id}>{p.name} · {rupiah(p.price)}/bulan</option>)}</select>
                  </label>
                  <div style={label}>Periode
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      {[1, 3, 12].map((m) => (
                        <button key={m} type="button" onClick={pickMonths(m)} style={{ height: 40, padding: '0 16px', border: 'none', borderRadius: 10, cursor: 'pointer', fontFamily: 'inherit', fontSize: 14, background: 'var(--surface)', color: 'var(--text)', boxShadow: f.months === m ? 'inset 0 0 0 2px var(--accent)' : 'none' }}>{m} bulan</button>
                      ))}
                    </div>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 15 }}><span>Total tagihan</span><b>{rupiah(billed)}</b></div>
                </div>

                <div style={box}>
                  <b style={{ fontSize: 14 }}>Transfer ke salah satu rekening</b>
                  {banks.length === 0 ? <span style={{ color: 'var(--muted)', fontSize: 14 }}>Rekening tujuan belum tersedia. Hubungi Convera.</span> : banks.map((b) => (
                    <label key={b.id} style={{ display: 'flex', gap: 10, alignItems: 'center', padding: 12, borderRadius: 10, background: 'var(--surface)', cursor: 'pointer', fontSize: 14 }}>
                      <input type="radio" name="bankAccount" checked={f.bankAccount === b.id} onChange={() => setF((x) => ({ ...x, bankAccount: b.id }))} />
                      <span><b>{b.bank}</b> · <span style={{ fontFamily: 'ui-monospace, Menlo, Consolas, monospace' }}>{b.number}</span><div style={{ color: 'var(--muted)', fontSize: 12 }}>a.n. {b.holder}</div></span>
                    </label>
                  ))}
                </div>

                <div style={box}>
                  <b style={{ fontSize: 14 }}>Data pengirim</b>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12 }}>
                    <label style={label}>Bank pengirim<input value={f.bank} onChange={set('bank')} maxLength={40} placeholder="mis. BCA" style={field} /></label>
                    <label style={label}>Nama pemilik rekening<input value={f.sender} onChange={set('sender')} maxLength={120} style={field} /></label>
                    <label style={label}>Nominal yang ditransfer (Rp)<input value={f.amount} onChange={set('amount')} inputMode="numeric" placeholder={String(billed)} style={field} /></label>
                    <label style={label}>No. referensi (opsional)<input value={f.ref} onChange={set('ref')} maxLength={60} style={field} /></label>
                  </div>
                  <label style={label}>Catatan (opsional)<input value={f.message} onChange={set('message')} maxLength={1000} style={field} /></label>
                  <label style={label}>Bukti transfer (JPG, PNG, atau PDF, maks. 5 MB)
                    <input type="file" accept=".jpg,.jpeg,.png,.pdf,image/jpeg,image/png,application/pdf" onChange={chooseFile} style={{ ...field, height: 'auto', padding: 10 }} />
                  </label>
                </div>
              </>
            )}

            {error ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{error}</div> : null}
            {pendingFull ? <div style={{ color: 'var(--muted)', fontSize: 13 }}>Sudah ada 3 bukti yang menunggu verifikasi. Tunggu hingga diperiksa.</div> : null}
            <button type="submit" disabled={busy || !plans.length || pendingFull} style={{ height: 46, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 600, cursor: busy ? 'default' : 'pointer', opacity: busy || pendingFull ? 0.6 : 1 }}>{busy ? 'Mengunggah…' : 'Kirim bukti transfer'}</button>

            {proofs.length ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <b style={{ fontSize: 14 }}>Riwayat bukti transfer</b>
                {proofs.slice(0, 5).map((p) => (
                  <div key={p.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 13, padding: '10px 12px', borderRadius: 10, background: 'var(--surface2)' }}>
                    <span>{new Date(p.created_at).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })} · {p.plan_name} · {p.months} bln · {rupiah(p.billed)}</span>
                    <span style={{ color: p.status === 'rejected' ? 'var(--danger)' : 'var(--muted)', textAlign: 'right' }}>{STATUS_ID[p.status]}{p.reject_reason ? ': ' + p.reject_reason : ''}</span>
                  </div>
                ))}
              </div>
            ) : null}
          </form>
        )}
      </section>
    </div>
  );
}
