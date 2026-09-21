package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateRow = errors.New("mensagem ja existe na base")
)

type InboxRow struct {
	id          uuid.UUID `db:"id"`
	msgId       string    `db:"msg_id"`
	consumerID  string    `db:"consumer_id"`
	payloadHash string    `db:"json_hash_hex"`
	receivedAt  time.Time `db:"received_at"`
	finishedAt  time.Time `db:"finished_at"`
}

type InboxRepository struct {
	pool *pgxpool.Pool
}

func NewInboxRepository(p *pgxpool.Pool) *InboxRepository {
	return &InboxRepository{pool: p}
}

func (i *InboxRepository) InsertInbox(ctx context.Context, tx pgx.Tx, id uuid.UUID, msg_id, consumer_id, payload string, receivedAt, finishedAt time.Time) error {

	_, err := tx.Exec(ctx, `INSERT INTO inbox( id , msg_id , consumer_id , json_hash_hex, received_at , finished_at) values ($1,$2,$3,$4,$5,$6)`, id, msg_id, consumer_id, payload, receivedAt, finishedAt)

	if err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) && pgError.Code == "23505" {
			return ErrDuplicateRow
		}
		return err
	}

	return nil
}
