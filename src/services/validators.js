export const USERNAME_RE = /^[a-z0-9][a-z0-9._-]{2,29}$/;
export const isUsername = (s) => USERNAME_RE.test(String(s).trim().toLowerCase());
export const isEmail = (s) => /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(String(s).trim());
export const normPhone = (p) => { let d = String(p).replace(/\D/g, ''); if (d.startsWith('0')) d = '62' + d.slice(1); else if (d.startsWith('8')) d = '62' + d; return d; };
export const isPhone = (s) => { const d = normPhone(s); return /^62\d{8,13}$/.test(d); };
export const isHttps = (u) => { try { return new URL(u).protocol === 'https:'; } catch { return false; } };
export function parseRecipients(str) {
  const out = { emails: [], phones: [], invalid: [] };
  String(str || '').split(/[,;\n]/).map((x) => x.trim()).filter(Boolean).forEach((x) => (isEmail(x) ? out.emails.push(x) : isPhone(x) ? out.phones.push(normPhone(x)) : out.invalid.push(x)));
  return out;
}
export function checkPassword(cur, next, again) {
  if (!cur) return 'Isi password saat ini';
  if (next.length < 8) return 'Password baru minimal 8 karakter';
  if (!/[A-Za-z]/.test(next) || !/\d/.test(next)) return 'Gunakan kombinasi huruf dan angka';
  if (next === cur) return 'Password baru tidak boleh sama dengan yang lama';
  if (next !== again) return 'Ulangi password baru dengan sama';
  return '';
}
export const isDatasetId = (s) => /^\d{15,16}$/.test(String(s).trim());
export const isToken = (s) => String(s).trim().length >= 20 && !/\s/.test(String(s).trim());
