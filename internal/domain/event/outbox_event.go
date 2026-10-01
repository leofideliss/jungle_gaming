package event

import (
	"time"

	"github.com/google/uuid"
)

type Event struct {
	EventID       uuid.UUID  `json:"eventId"`
	EventType     string     `json:"eventType"`
	AggregateID   uuid.UUID  `json:"aggregateId"`
	AggregateType string     `json:"aggregateType"`
	CorrelationID string     `json:"correlationId"`
	CausationID   *uuid.UUID `json:"causationId,omitempty"`
	OccurredAt    time.Time  `json:"occurredAt"`
	Version       int        `json:"version"`
	Data          any        `json:"data"`
}

type WagerTransactionProcessedData struct {
	TransactionID string `json:"transactionId"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	ProviderID    string `json:"providerId"`
	PlayerID      string `json:"playerId"`
	WalletID      string `json:"walletId"`
	ResultBalance string `json:"resultBalance"`
}

type WagerTransactionRejectedData struct {
	TransactionID string `json:"transactionId"`
	Kind          string `json:"kind"`
	FailureCode   string `json:"failureCode"`
	ProviderID    string `json:"providerId"`
}

type WalletBalanceChangedData struct {
	WalletID      string `json:"walletId"`
	TransactionID string `json:"transactionId"`
	Direction     string `json:"direction"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	BalanceBefore string `json:"balanceBefore"`
	BalanceAfter  string `json:"balanceAfter"`
	WalletVersion int64  `json:"walletVersion"`
}

type WagerTransactionPendingReferenceData struct {
	TransactionID       string `json:"transactionId"`
	ReferenceExternalID string `json:"referenceExternalId"`
	ProviderID          string `json:"providerId"`
}

func NewWagerTransactionProcessed(correlationID string, data WagerTransactionProcessedData) Event {
	return Event{
		EventID:       uuid.Must(uuid.NewV7()),
		EventType:     "WagerTransactionProcessed",
		AggregateType: "WagerTransaction",
		CorrelationID: correlationID,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Data:          data,
	}
}

func NewWagerTransactionPendingReference(correlationID string, data WagerTransactionPendingReferenceData) Event {
	return Event{
		EventID:       uuid.Must(uuid.NewV7()),
		EventType:     "WagerTransactionPendingReference",
		AggregateType: "WagerTransaction",
		CorrelationID: correlationID,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Data:          data,
	}
}

func NewWalletBalanceChanged(correlationID string, causationID *uuid.UUID, data WalletBalanceChangedData) Event {
	return Event{
		EventID:       uuid.Must(uuid.NewV7()),
		EventType:     "WalletBalanceChanged",
		AggregateType: "WalletBalance",
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Data:          data,
	}
}

func NewWagerTransactionRejected(correlationID string, data WagerTransactionRejectedData) Event {
	return Event{
		EventID:       uuid.Must(uuid.NewV7()),
		EventType:     "WagerTransactionRejected",
		AggregateType: "WagerTransaction",
		CorrelationID: correlationID,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Data:          data,
	}
}
