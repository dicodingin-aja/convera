package billing

// EffectiveStatusSQL menghitung status langganan yang berlaku sekarang untuk baris `subscriptions` ber-alias `sub`.
// Kolom status bisa tertinggal (tugas kedaluwarsa berjalan berkala), jadi tampilan selalu menghitung ulang:
//   - active tetap active selama masa tenggang (settings.grace_days) setelah current_period_end
//   - trial yang trial_until-nya lewat dianggap expired
const EffectiveStatusSQL = `CASE
  WHEN sub.status IS NULL THEN 'none'
  WHEN sub.status = 'active' AND sub.current_period_end IS NOT NULL
       AND sub.current_period_end + make_interval(days => COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'grace_days'), 3)) < now() THEN 'expired'
  WHEN sub.status = 'trial' AND sub.trial_until IS NOT NULL AND sub.trial_until < now() THEN 'expired'
  ELSE sub.status END`

// GraceSQL: masa tenggang setelah current_period_end (settings.grace_days, default 3 hari).
const GraceSQL = `make_interval(days => COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'grace_days'), 3))`

// AccessSQL bernilai true bila workspace (baris `subscriptions` ber-alias `sub`) boleh memakai fitur Convera:
// trial yang masih berjalan, atau langganan aktif termasuk masa tenggang. Status 'pending' (bukti transfer
// menunggu verifikasi) tetap boleh selama trial atau periode sebelumnya belum habis, supaya customer yang
// membayar lebih awal tidak terkunci. Selain itu (belum bayar, expired, stopped, tanpa baris) bernilai false.
const AccessSQL = `COALESCE(CASE sub.status
  WHEN 'trial'   THEN sub.trial_until IS NULL OR sub.trial_until > now()
  WHEN 'active'  THEN sub.current_period_end IS NULL OR sub.current_period_end + ` + GraceSQL + ` > now()
  WHEN 'pending' THEN COALESCE(sub.trial_until > now(), false) OR COALESCE(sub.current_period_end + ` + GraceSQL + ` > now(), false)
  ELSE false END, false)`
