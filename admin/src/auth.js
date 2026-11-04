// Login admin tunggal. Mode demo: belum ada server, jadi kredensial TIDAK diverifikasi sungguhan.
// Saat backend siap, ganti isi login()/logout() dengan POST/DELETE ke endpoint sesi admin.
const KEY = 'convera-admin:session';

export const getSession = () => { try { return JSON.parse(localStorage.getItem(KEY)); } catch { return null; } };

export async function login(username, password) {
  if (String(username).trim().toLowerCase() !== 'admin') throw new Error('Username atau password salah');
  if (String(password).length < 6) throw new Error('Username atau password salah');
  const session = { user: 'admin', at: new Date().toISOString() };
  try { localStorage.setItem(KEY, JSON.stringify(session)); } catch { /* abaikan */ }
  return session;
}

export const logout = () => { try { localStorage.removeItem(KEY); } catch { /* abaikan */ } };
