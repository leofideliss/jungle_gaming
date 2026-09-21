package wager

import (
	"errors"
	"testing"
	"time"

	"jungle_gaming/internal/domain/money"

	"github.com/google/uuid"
)

func setupWagerTransaction(t *testing.T, status Status) WagerTransaction {
	t.Helper()

	w, err := RestoreWagerTransaction(RestoreWagerTransactionInput{
		Id:                  uuid.Must(uuid.NewV7()),
		ExternalTrId:        "ext_tx_987654321",
		ProviderID:          "provider_evolution_gaming",
		IdempotencyKey:      "idem_key_abc123xyz",
		WalletId:            uuid.Must(uuid.NewV7()),
		PlayerId:            uuid.Must(uuid.NewV7()),
		GameId:              "game_starburst_01",
		RoundID:             "round_998877",
		ReferenceExternalId: "ref_ext_001122",
		ReferenceInternalId: uuid.Must(uuid.NewV7()),
		PayloadHash:         "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Kind:                KindBet,
		Status:              status,
		CreatedAt:           time.Now().Add(-1 * time.Minute),
		UpdatedAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("setup falhou: %v", err)
	}
	return w
}

func TestMarkAsProcessed(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		wantResult Status
		wantErr    error
	}{
		{name: "PENDING para PROCESSED", status: StatusPending, wantResult: StatusProcessed},
		{name: "PENDING_REFERENCE para PROCESSED", status: StatusPendingReference, wantResult: StatusProcessed},
		{name: "REJECTED nao processa", status: StatusRejected, wantErr: ErrInvalidChangeStatus},
		{name: "PROCESSED nao reprocessa", status: StatusProcessed, wantErr: ErrInvalidChangeStatus},
		{name: "FAILED nao processa", status: StatusFailed, wantErr: ErrInvalidChangeStatus},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := setupWagerTransaction(t, test.status)
			err := w.MarkAsProcessed(money.Money{})

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("erro esperado %v, recebido %v", test.wantErr, err)
				}
				if w.status != test.status {
					t.Errorf("transicao bloqueada mudou o status: era %s, virou %s", test.status, w.status)
				}
				return
			}

			if err != nil {
				t.Fatalf("erro nao esperado: %v", err)
			}
			if w.status != test.wantResult {
				t.Errorf("status esperado %s, recebido %s", test.wantResult, w.status)
			}
		})
	}
}

func TestMarkAsRejected(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		wantResult Status
		wantErr    error
	}{
		{name: "PENDING para REJECTED", status: StatusPending, wantResult: StatusRejected},
		{name: "PENDING_REFERENCE para REJECTED", status: StatusPendingReference, wantResult: StatusRejected},
		{name: "REJECTED nao rejeita de novo", status: StatusRejected, wantErr: ErrInvalidChangeStatus},
		{name: "PROCESSED nao rejeita", status: StatusProcessed, wantErr: ErrInvalidChangeStatus},
		{name: "FAILED nao rejeita", status: StatusFailed, wantErr: ErrInvalidChangeStatus},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := setupWagerTransaction(t, test.status)
			err := w.MarkAsRejected("INSUFFICIENT_BALANCE")

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("erro esperado %v, recebido %v", test.wantErr, err)
				}
				if w.status != test.status {
					t.Errorf("transicao bloqueada mudou o status: era %s, virou %s", test.status, w.status)
				}
				return
			}

			if err != nil {
				t.Fatalf("erro nao esperado: %v", err)
			}
			if w.status != test.wantResult {
				t.Errorf("status esperado %s, recebido %s", test.wantResult, w.status)
			}
		})
	}
}

func TestMarkAsPendingReference(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		wantResult Status
		wantErr    error
	}{
		{name: "PENDING para PENDING_REFERENCE", status: StatusPending, wantResult: StatusPendingReference},
		{name: "PENDING_REFERENCE nao repete", status: StatusPendingReference, wantErr: ErrInvalidChangeStatus},
		{name: "PROCESSED nao vira pending_reference", status: StatusProcessed, wantErr: ErrInvalidChangeStatus},
		{name: "REJECTED nao vira pending_reference", status: StatusRejected, wantErr: ErrInvalidChangeStatus},
		{name: "FAILED nao vira pending_reference", status: StatusFailed, wantErr: ErrInvalidChangeStatus},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := setupWagerTransaction(t, test.status)
			err := w.MarkAsPendingReference()

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("erro esperado %v, recebido %v", test.wantErr, err)
				}
				if w.status != test.status {
					t.Errorf("transicao bloqueada mudou o status: era %s, virou %s", test.status, w.status)
				}
				return
			}

			if err != nil {
				t.Fatalf("erro nao esperado: %v", err)
			}
			if w.status != test.wantResult {
				t.Errorf("status esperado %s, recebido %s", test.wantResult, w.status)
			}
		})
	}
}

func TestMarkAsFailed(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		wantResult Status
		wantErr    error
	}{
		{name: "PENDING para FAILED", status: StatusPending, wantResult: StatusFailed},
		{name: "PENDING_REFERENCE para FAILED", status: StatusPendingReference, wantResult: StatusFailed},
		{name: "PROCESSED nao falha", status: StatusProcessed, wantErr: ErrInvalidChangeStatus},
		{name: "REJECTED nao falha", status: StatusRejected, wantErr: ErrInvalidChangeStatus},
		{name: "FAILED nao falha de novo", status: StatusFailed, wantErr: ErrInvalidChangeStatus},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := setupWagerTransaction(t, test.status)
			err := w.MarkAsFailed("TECHNICAL_ERROR")

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("erro esperado %v, recebido %v", test.wantErr, err)
				}
				if w.status != test.status {
					t.Errorf("transicao bloqueada mudou o status: era %s, virou %s", test.status, w.status)
				}
				return
			}

			if err != nil {
				t.Fatalf("erro nao esperado: %v", err)
			}
			if w.status != test.wantResult {
				t.Errorf("status esperado %s, recebido %s", test.wantResult, w.status)
			}
		})
	}
}
