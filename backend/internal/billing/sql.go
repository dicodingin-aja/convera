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
