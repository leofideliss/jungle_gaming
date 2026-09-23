package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"jungle_gaming/internal/domain/wager"

	"github.com/google/uuid"
)

var (
	ErrInvalidPayloadHash = errors.New("payload invalido")
)

type PayloadHash struct {
	Kind                wager.Kind `json:"kind"`
	PlayerID            uuid.UUID  `json:"playerId"`
	WalletID            uuid.UUID  `json:"walletId"`
	RoundID             string     `json:"roundId"`
	GameID              string     `json:"gameId"`
	ReferenceExternalID string     `json:"referenceExternalId"`
	Amount              string     `json:"amount"`
}

func NewPayloadHash(k wager.Kind, playerId, walletId uuid.UUID, roundId, gameId, referencExtId string, amount string) PayloadHash {
	return PayloadHash{
		Kind:                k,
		PlayerID:            playerId,
		WalletID:            walletId,
		RoundID:             roundId,
		GameID:              gameId,
		ReferenceExternalID: referencExtId,
		Amount:              amount,
	}
}

func (p PayloadHash) CalculatePayloadHash() (string, error) {
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return "", ErrInvalidPayloadHash
	}

	hash := sha256.Sum256(payloadBytes)
	return hex.EncodeToString(hash[:]), nil
}
