package money

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

type Money struct {
	amount   int64
	currency string
}

var supportedCurrencies = map[string]bool{
	"BRL": true,
}

var (
	ErrEmptyAmount      = errors.New("valor vazio")
	ErrFormatInvalid    = errors.New("formato inválido")
	ErrNegativeAmount   = errors.New("valor negativo não permitido")
	ErrScaleExceeded    = errors.New("escala excede 2 casas decimais")
	ErrAmountOverflow   = errors.New("valor excede o limite suportado")
	ErrInvalidCurrency  = errors.New("moeda inválida")
	ErrCurrencyMismatch = errors.New("moedas diferentes")
)

func NewMoney(amount string, currency string) (Money, error) {

	value, err := parseToInt(amount)
	if err != nil {
		return Money{}, err
	}

	errCurrency := validateCurrency(currency)
	if errCurrency != nil {
		return Money{}, errCurrency
	}
	return Money{
		amount:   value,
		currency: currency,
	}, nil
}

func RestoreMoney(amount int64, currency string) (Money, error) {
	return newFromCents(amount, currency)
}

func newFromCents(amount int64, currency string) (Money, error) {
	err := validateCurrency(currency)
	if err != nil {
		return Money{}, ErrInvalidCurrency
	}

	return Money{amount: amount, currency: currency}, nil
}

func (m Money) Add(otherValue Money) (Money, error) {
	if m.currency != otherValue.currency {
		return Money{}, ErrCurrencyMismatch
	}

	if otherValue.amount > 0 && m.amount > math.MaxInt64-otherValue.amount {
		return Money{}, ErrAmountOverflow
	}

	if otherValue.amount < 0 && m.amount < math.MinInt64-otherValue.amount {
		return Money{}, ErrAmountOverflow
	}

	return Money{
		amount:   m.amount + otherValue.amount,
		currency: m.currency,
	}, nil
}

func (m Money) Sub(otherValue Money) (Money, error) {
	nagativeValue, err := otherValue.Negate()
	if err != nil {
		return Money{}, err
	}

	newValue, err := m.Add(nagativeValue)

	if err != nil {
		return Money{}, err
	}

	return newValue, nil
}

func (m Money) Negate() (Money, error) {

	if m.amount == math.MinInt64 {
		return Money{}, ErrAmountOverflow
	}

	return Money{
		amount:   -m.amount,
		currency: m.currency,
	}, nil
}

func (m Money) Compare(otherValue Money) (int, error) {

	if m.currency != otherValue.currency {
		return 0, ErrCurrencyMismatch
	}

	if m.amount < otherValue.amount {
		return -1, nil
	}

	if m.amount > otherValue.amount {
		return 1, nil
	}
	return 0, nil
}

func Zero(currency string) (Money, error) {
	return newFromCents(0, currency)
}

func (m Money) String() string {
	var sinal string
	value := m.amount
	if value < 0 {
		sinal = "-"
		value = -value
	}

	splitInteger := value / 100
	splitDecimal := value % 100

	strInteira := strconv.FormatInt(splitInteger, 10)
	strDecimal := strconv.FormatInt(splitDecimal, 10)

	strDecimal = strings.Repeat("0", 2-len(strDecimal)) + strDecimal

	return sinal + strInteira + "." + strDecimal
}

func (m Money) IsNegative() bool {
	return m.amount < 0
}

func (m Money) IsPositive() bool {
	return m.amount > 0
}

func (m Money) IsZero() bool {
	return m.amount == 0
}

func (m Money) Currency() string {
	return m.currency
}

func (m Money) Cents() int64 {
	return m.amount
}

func validateCurrency(c string) error {
	for _, ch := range c {
		if ch < 'A' || ch > 'Z' {
			return ErrInvalidCurrency
		}
	}

	if !supportedCurrencies[c] {
		return ErrInvalidCurrency
	}

	return nil
}

func parseToInt(v string) (int64, error) {
	if v == "" {
		return 0, ErrEmptyAmount
	}

	if strings.HasPrefix(v, "-") {
		return 0, ErrNegativeAmount
	}

	split := strings.Split(v, ".")
	if len(split) > 2 {
		return 0, ErrFormatInvalid
	}

	splitInteger := split[0]
	var splitDecimal string
	if len(split) == 2 {
		splitDecimal = split[1]
	}

	if len(splitDecimal) > 2 {
		return 0, ErrScaleExceeded
	}

	splitDecimal = splitDecimal + strings.Repeat("0", 2-len(splitDecimal))

	digits := splitInteger + splitDecimal
	cents, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, ErrAmountOverflow
		}
		return 0, ErrFormatInvalid
	}

	return cents, nil
}
