package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type outboxRow struct {
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
