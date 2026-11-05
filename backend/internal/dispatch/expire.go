package dispatch

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpireSubscriptions mengubah langganan yang lewat masa tenggang (active) atau masa trial (trial) menjadi
// 'expired' dan memberi tahu pemilik workspace. Idempoten; aman dipanggil berkala.
func ExpireSubscriptions(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	tag, err := db.Exec(ctx, `
WITH done AS (
  UPDATE subscriptions SET status = 'expired'
  WHERE (status = 'active' AND current_period_end IS NOT NULL
         AND current_period_end + make_interval(days => COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'grace_days'), 3)) < now())
     OR (status = 'trial' AND trial_until IS NOT NULL AND trial_until < now())
  RETURNING workspace_id
)
INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
SELECT w.owner_user_id, w.id, 'subscription_expired', 'Langganan berakhir',
       'Perpanjang langganan agar event WhatsApp tetap terkirim ke Meta.', '/langganan'
FROM done JOIN workspaces w ON w.id = done.workspace_id`)
	return tag.RowsAffected(), err
}

// RunExpiry menjalankan ExpireSubscriptions tiap interval sampai ctx dibatalkan.
func RunExpiry(ctx context.Context, db *pgxpool.Pool, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := ExpireSubscriptions(ctx, db); err != nil && ctx.Err() == nil {
			log.Printf("expire: %v", err)
		} else if n > 0 {
			log.Printf("expire: %d langganan berakhir", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
