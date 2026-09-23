package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRowsNotAffected = errors.New("operacao no BD falhou")
)

type OutboxRow struct {
	ID            uuid.UUID
	EventID       uuid.UUID
	AggregateID   uuid.UUID
	AggregateType string
	EventType     string
	Payload       []byte
	OccurredAt    time.Time
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	PublishedAt   *time.Time
	LockedBy      *string
	LockedAt      *time.Time
	CreatedAt     time.Time
}

type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(p *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: p}
}

func (o *OutboxRepository) Insert(ctx context.Context, tx pgx.Tx, eventId, aggregateId uuid.UUID, eventType, aggregateType string, payload []byte) error {
	id := uuid.Must(uuid.NewV7())

	_, err := tx.Exec(ctx, `INSERT INTO outbox (id, event_id , aggregate_id , aggregate_type , event_type , payload ) VALUES($1,$2,$3,$4,$5,$6)`, id, eventId, aggregateId, aggregateType, eventType, payload)

	if err != nil {
		return err
	}

	return nil
}

func (o *OutboxRepository) FetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]OutboxRow, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, event_id, aggregate_id, aggregate_type, event_type, payload,
		        occurred_at, status, attempts, next_attempt_at, published_at,
		        locked_by, locked_at, created_at
		   FROM outbox
		  WHERE status = 'PENDING' AND next_attempt_at <= NOW()
		  ORDER BY created_at
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OutboxRow
	for rows.Next() {
		var row OutboxRow
		err := rows.Scan(
			&row.ID, &row.EventID, &row.AggregateID, &row.AggregateType, &row.EventType, &row.Payload,
			&row.OccurredAt, &row.Status, &row.Attempts, &row.NextAttemptAt, &row.PublishedAt,
			&row.LockedBy, &row.LockedAt, &row.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (o *OutboxRepository) MarkAsPublished(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	row, err := tx.Exec(ctx, `UPDATE outbox SET status = 'PUBLISHED' WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if row.RowsAffected() == 0 {
		return ErrRowsNotAffected
	}

	return nil
}

func (o *OutboxRepository) MarkAsFailed(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	row, err := tx.Exec(ctx, `UPDATE outbox SET status = 'FAILED' WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if row.RowsAffected() == 0 {
		return ErrRowsNotAffected
	}

	return nil
}
