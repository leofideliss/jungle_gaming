package idempotency

import (
	"jungle_gaming/internal/domain/wager"
	"testing"

	"github.com/google/uuid"
)

func TestCalculatePayloadHash(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())

	base := NewPayloadHash(wager.KindBet, uid, uid, "round-123", "game-123", "ref-123", "25.00")
	same := NewPayloadHash(wager.KindBet, uid, uid, "round-123", "game-123", "ref-123", "25.00")
	diffAmount := NewPayloadHash(wager.KindBet, uid, uid, "round-123", "game-123", "ref-123", "30.00")

	h1, err := base.CalculatePayloadHash()
	if err != nil {
		t.Fatalf("hash base: %v", err)
	}
	h2, _ := same.CalculatePayloadHash()
	h3, _ := diffAmount.CalculatePayloadHash()

	if h1 != h2 {
		t.Errorf("mesmo conteudo deveria dar mesmo hash: %s vs %s", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("amount diferente deveria dar hash diferente, mas deu igual: %s", h1)
	}
	if len(h1) != 64 {
		t.Errorf("hash deveria ter 64 chars (sha256 hex), tem %d", len(h1))
	}
}
