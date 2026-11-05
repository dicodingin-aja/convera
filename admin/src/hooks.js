import { useCallback, useEffect, useState } from 'react';
import { api } from './api.js';

// Mengambil data dari API. path = null menunda pengambilan. reload() mengambil ulang.
export function useData(path) {
  const [st, setSt] = useState({ data: null, error: '', loading: !!path });
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!path) { setSt({ data: null, error: '', loading: false }); return undefined; }
    let alive = true;
    setSt((s) => ({ ...s, loading: true, error: '' }));
    api.get(path)
      .then((r) => { if (alive) setSt({ data: r, error: '', loading: false }); })
      .catch((e) => { if (alive) setSt((s) => ({ data: s.data, error: e.message, loading: false })); });
    return () => { alive = false; };
  }, [path, tick]);
  const reload = useCallback(() => setTick((n) => n + 1), []);
  return { ...st, reload };
}

// Menjalankan aksi (POST/PUT/DELETE) dengan status sibuk + pesan error.
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const run = useCallback(async (fn) => {
    setBusy(true); setError('');
    try { return await fn(); } catch (e) { setError(e.message); return undefined; } finally { setBusy(false); }
  }, []);
  return { busy, error, setError, run };
}

export function useDebounced(value, ms = 300) {
  const [v, setV] = useState(value);
  useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
  return v;
}
