package currency

import (
	"math"
	"testing"
)

func TestConvertUSD(t *testing.T) {
	tests := []struct {
		amount   float64
		currency Code
		expected float64
		epsilon  float64 // allow for floating-point rounding
	}{
		{100, USD, 100.0, 0.01},
		{100, EUR, 92.0, 0.01},
		{100, GBP, 79.0, 0.01},
		{100, JPY, 15450.0, 1.0},
		{1000, CAD, 1370.0, 0.01},
		{50, AUD, 76.0, 0.01},
	}

	for _, tc := range tests {
		t.Run(string(tc.currency), func(t *testing.T) {
			result, err := ConvertUSD(tc.amount, tc.currency)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(result-tc.expected) > tc.epsilon {
				t.Errorf("expected %f, got %f (epsilon=%f)", tc.expected, result, tc.epsilon)
			}
		})
	}
}

func TestConvertUSD_InvalidCurrency(t *testing.T) {
	_, err := ConvertUSD(100, Code("INVALID"))
	if err == nil {
		t.Fatal("expected error for invalid currency")
	}
}

func TestConvertFrom(t *testing.T) {
	tests := []struct {
		amount       float64
		fromCurrency Code
		toCurrency   Code
		expected     float64
		epsilon      float64
	}{
		{100, USD, USD, 100.0, 0.01},     // same currency
		{100, EUR, USD, 108.7, 0.5},      // EUR -> USD
		{1000, JPY, USD, 6.48, 0.1},      // JPY -> USD (approx)
		{100, USD, EUR, 92.0, 0.1},       // USD -> EUR
		{100, EUR, GBP, 85.87, 1.0},      // EUR -> GBP (via USD)
	}

	for _, tc := range tests {
		t.Run(string(tc.fromCurrency)+"_to_"+string(tc.toCurrency), func(t *testing.T) {
			result, err := ConvertFrom(tc.amount, tc.fromCurrency, tc.toCurrency)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(result-tc.expected) > tc.epsilon {
				t.Errorf("expected %f, got %f (tolerance=%f)", tc.expected, result, tc.epsilon)
			}
		})
	}
}

func TestConvertFrom_InvalidSource(t *testing.T) {
	_, err := ConvertFrom(100, Code("INVALID"), USD)
	if err == nil {
		t.Fatal("expected error for invalid source currency")
	}
}

func TestConvertFrom_InvalidTarget(t *testing.T) {
	_, err := ConvertFrom(100, USD, Code("INVALID"))
	if err == nil {
		t.Fatal("expected error for invalid target currency")
	}
}

func TestFormatSymbol(t *testing.T) {
	tests := []struct {
		code     Code
		expected string
	}{
		{USD, "$"},
		{EUR, "€"},
		{GBP, "£"},
		{JPY, "¥"},
		{CAD, "C$"},
		{AUD, "A$"},
		{Code("UNKNOWN"), "UNKNOWN"},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			result := FormatSymbol(tc.code)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		amount   float64
		currency Code
		expected string
	}{
		{1255.46, USD, "$1255.46"},
		{1255.46, EUR, "€1255.46"},
		{15450.4, JPY, "¥15450"}, // rounds to nearest integer
		{1255.46, GBP, "£1255.46"},
	}

	for _, tc := range tests {
		t.Run(string(tc.currency), func(t *testing.T) {
			result := Format(tc.amount, tc.currency)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestIsValidCode(t *testing.T) {
	if !IsValidCode(USD) {
		t.Error("USD should be valid")
	}
	if !IsValidCode(EUR) {
		t.Error("EUR should be valid")
	}
	if IsValidCode(Code("NOTREAL")) {
		t.Error("NOTREAL should not be valid")
	}
}

func TestAllCodes(t *testing.T) {
	codes := AllCodes()
	if len(codes) == 0 {
		t.Fatal("expected at least one currency code")
	}
	// Verify USD is in the list
	found := false
	for _, code := range codes {
		if code == USD {
			found = true
			break
		}
	}
	if !found {
		t.Error("USD not found in AllCodes()")
	}
}

func TestExchangeRates_Consistency(t *testing.T) {
	// Verify that USD rate is always 1.0
	if ExchangeRates[USD] != 1.0 {
		t.Errorf("USD rate should be 1.0, got %f", ExchangeRates[USD])
	}

	// Verify all rates are positive
	for code, rate := range ExchangeRates {
		if rate <= 0 {
			t.Errorf("currency %s has non-positive rate: %f", code, rate)
		}
	}
}

func BenchmarkConvertUSD(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ConvertUSD(1000, EUR)
	}
}

func BenchmarkConvertFrom(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ConvertFrom(1000, EUR, GBP)
	}
}
