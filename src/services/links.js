// Tautan dari email membawa token di query string (?reset=... / ?verify=...). Token diambil sekali lalu dihapus
// dari URL agar tidak tertinggal di riwayat browser, bookmark, atau header Referer.
export function takeParam(name) {
  try {
    const u = new URL(window.location.href);
    const v = u.searchParams.get(name);
    if (v) {
      u.searchParams.delete(name);
      window.history.replaceState({}, '', u.pathname + u.search + u.hash);
    }
    return v || '';
  } catch { return ''; }
}
