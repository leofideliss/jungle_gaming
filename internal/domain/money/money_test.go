package money

import (
	"errors"
	"math"
	"testing"
)

func TestNewMoney(t *testing.T) {
	tests := []struct {
		name      string
		amount    string
		currency  string
		wantCents int64
		wantErr   error
	}{
		// sucessos
		{name: "duas casas decimais", amount: "25.00", currency: "BRL", wantCents: 2500},
		{name: "uma casa recebe padding", amount: "25.5", currency: "BRL", wantCents: 2550},
		{name: "sem parte decimal", amount: "25", currency: "BRL", wantCents: 2500},
		{name: "um centavo", amount: "0.01", currency: "BRL", wantCents: 1},
		{name: "zero", amount: "0.00", currency: "BRL", wantCents: 0},

		// erros de valor
		{name: "vazio", amount: "", currency: "BRL", wantErr: ErrEmptyAmount},
		{name: "negativo", amount: "-5.00", currency: "BRL", wantErr: ErrNegativeAmount},
		{name: "escala excedente", amount: "25.005", currency: "BRL", wantErr: ErrScaleExceeded},
		{name: "pontos demais", amount: "25.00.00", currency: "BRL", wantErr: ErrFormatInvalid},
		{name: "lixo textual", amount: "abc", currency: "BRL", wantErr: ErrFormatInvalid},
		{name: "notacao cientifica", amount: "2e3", currency: "BRL", wantErr: ErrFormatInvalid},
		{name: "overflow", amount: "99999999999999999999.99", currency: "BRL", wantErr: ErrAmountOverflow},

		// erros de moeda
		{name: "moeda vazia", amount: "25.00", currency: "", wantErr: ErrInvalidCurrency},
		{name: "moeda com duas letras", amount: "25.00", currency: "US", wantErr: ErrInvalidCurrency},
		{name: "moeda nao suportada", amount: "25.00", currency: "XYZ", wantErr: ErrInvalidCurrency},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewMoney(tc.amount, tc.currency)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("esperava erro %v, recebi %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("nao esperava erro, recebi %v", err)
			}
			if m.amount != tc.wantCents {
				t.Errorf("amount = %d, esperado %d", m.amount, tc.wantCents)
			}
		})
	}
}

func TestNewFromCents(t *testing.T) {
	tests := []struct {
		name      string
		amount    int64
		currency  string
		wantCents int64
		wantErr   error
	}{
		{name: "valor inciado do banco", amount: 2550, currency: "BRL", wantCents: 2550},

		// erros
		{name: "valor inciado do banco sem moeada", amount: 2550, currency: "", wantCents: 2550, wantErr: ErrInvalidCurrency},
		{name: "valor inciado do banco moeada invalida", amount: 2550, currency: "XSWE", wantCents: 2550, wantErr: ErrInvalidCurrency},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, err := newFromCents(test.amount, test.currency)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("esperava o erro %v , recebu %v", test.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("nao esperava erro , recebi %v", err)
			}

			if m.amount != test.wantCents {
				t.Errorf("amount recebido %d , amount esperado %d", m.amount, test.wantCents)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name         string
		currentValue string
		otherValue   string
		resultValue  int64
		wantErr      error
	}{
		{name: "dez mais dez", currentValue: "10.00", otherValue: "10.00", resultValue: 2000},
		{name: "com centavos", currentValue: "10.50", otherValue: "5.25", resultValue: 1575},
		{name: "soma com zero", currentValue: "10.00", otherValue: "0.00", resultValue: 1000},
		{name: "gera carry de centavos", currentValue: "0.99", otherValue: "0.01", resultValue: 100},
		{name: "valores grandes", currentValue: "99999.99", otherValue: "0.01", resultValue: 10000000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m1, err := NewMoney(test.currentValue, "BRL")
			if err != nil {
				t.Fatalf("Setup Falhou (%v)", err)
			}

			m2, err := NewMoney(test.otherValue, "BRL")
			if err != nil {
				t.Fatalf("Setup Falhou (%v)", err)
			}

			m3, err := m1.Add(m2)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error (%v) , esperava o erro (%v) ", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("nao esperava erro e recebi %v", err)
			}

			if m3.amount != test.resultValue {
				t.Errorf("resultado da soma esperava %d e recebu %d", test.resultValue, m3.amount)
			}
		})
	}
}

func TestAddOverflow(t *testing.T) {
	m1, err := newFromCents(math.MaxInt64, "BRL")
	if err != nil {
		t.Fatalf("setup falhou: %v", err)
	}
	m2, err := newFromCents(1, "BRL")
	if err != nil {
		t.Fatalf("setup falhou: %v", err)
	}

	_, err = m1.Add(m2)
	if !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("esperava ErrAmountOverflow, recebi %v", err)
	}
}

func TestNegate(t *testing.T) {
	tests := []struct {
		name       string
		amount     string
		wantAmount int64
		wantErr    error
	}{
		{name: "10 para -10", amount: "10", wantAmount: -1000},
		{name: "zero permanece zero", amount: "0.00", wantAmount: 0},
		{name: "valor com centavos", amount: "10.50", wantAmount: -1050},
		{name: "um centavo", amount: "0.01", wantAmount: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			money, err := NewMoney(test.amount, "BRL")

			if err != nil {
				t.Fatalf("Erro ao criar moeda %v", err)
			}

			negativeValue, err := money.Negate()

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Erro esperado %v erro que recebi %v", test.wantErr, err)

				}
				return
			}

			if err != nil {
				t.Fatalf("Erro ( %v )", err)

			}

			if negativeValue.amount != test.wantAmount {
				t.Errorf("valor esperado era %d e recebi %d", test.wantAmount, negativeValue.amount)
			}

		})
	}
}

func TestNegateOverflow(t *testing.T) {
	m, err := newFromCents(math.MinInt64, "BRL")
	if err != nil {
		t.Fatalf("Erro ao criar moeda %v", err)
	}

	_, err = m.Negate()
	if !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("esperava ErrAmountOverflow, recebi %v", err)
	}
}

func TestMoneyToString(t *testing.T) {
	tests := []struct {
		name       string
		value      int64
		wantResult string
	}{
		{name: "convertendo 1000 para 10.00", value: 1000, wantResult: "10.00"},
		{name: "padding de centavos", value: 2505, wantResult: "25.05"},
		{name: "um centavo", value: 1, wantResult: "0.01"},
		{name: "zero", value: 0, wantResult: "0.00"},
		{name: "valor negativo", value: -2500, wantResult: "-25.00"},
		{name: "negativo com centavo", value: -1, wantResult: "-0.01"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			money, err := newFromCents(test.value, "BRL")
			if err != nil {
				t.Fatalf("Erro no setup %v", err)
			}

			moneyString := money.String()

			if moneyString != test.wantResult {
				t.Errorf("valor esperado %q valor recebido %q", test.wantResult, moneyString)
			}
		})
	}

}
