package repository

import "context"

// A single transaction makes inbox insertion and acknowledgment atomic. Any
// error/timeout rolls back; the durable intent is picked up on the next poll.
// SKIP LOCKED permits multiple workers without duplicate delivery.
func (s *Store) Deliver(ctx context.Context) (int, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer rollback(tx)
	tag, e := tx.Exec(ctx, `WITH batch AS (
 SELECT event_id,recipient_id FROM hostelhive.notification_outbox WHERE delivered_at IS NULL ORDER BY event_id,recipient_id LIMIT 100 FOR UPDATE SKIP LOCKED
 ), inserted AS (
 INSERT INTO hostelhive.notifications(event_id,recipient_id) SELECT event_id,recipient_id FROM batch
 ON CONFLICT(event_id,recipient_id) DO NOTHING RETURNING event_id,recipient_id
 )
 UPDATE hostelhive.notification_outbox o SET delivered_at=clock_timestamp() FROM batch b WHERE o.event_id=b.event_id AND o.recipient_id=b.recipient_id`)
	if e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return int(tag.RowsAffected()), nil
}
