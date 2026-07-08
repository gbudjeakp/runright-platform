// Package currency provides utilities for multi-currency pricing and conversion.
package currency

import (
	"fmt"
)

// Code represents an ISO 4217 currency code.
type Code string

const (
	USD Code = "USD" // US Dollar
	EUR Code = "EUR" // Euro
	GBP Code = "GBP" // British Pound
	JPY Code = "JPY" // Japanese Yen
	CAD Code = "CAD" // Canadian Dollar
	AUD Code = "AUD" // Australian Dollar
	CNY Code = "CNY" // Chinese Yuan
	INR Code = "INR" // Indian Rupee
)

// ExchangeRates contains conversion rates from USD to other currencies.
// These are representative mid-market rates as of 2026-06-23.
// In production, these should be fetched from an external API (e.g., fixer.io, exchangerate-api.com).
var ExchangeRates = map[Code]float64{
	USD: 1.0,
	EUR: 0.92,  // 1 USD = 0.92 EUR
	GBP: 0.79,  // 1 USD = 0.79 GBP
	JPY: 154.5, // 1 USD = 154.5 JPY
	CAD: 1.37,  // 1 USD = 1.37 CAD
	AUD: 1.52,  // 1 USD = 1.52 AUD
	CNY: 7.24,  // 1 USD = 7.24 CNY
	INR: 83.12, // 1 USD = 83.12 INR
}

// ConvertUSD converts an amount from USD to the target currency.
func ConvertUSD(amountUSD float64, targetCurrency Code) (float64, error) {
	rate, ok := ExchangeRates[targetCurrency]
	if !ok {
		return 0, fmt.Errorf("unknown currency code: %s", targetCurrency)
	}
	return amountUSD * rate, nil
}

// ConvertFrom converts an amount from one currency to another.
func ConvertFrom(amount float64, fromCurrency, toCurrency Code) (float64, error) {
	if fromCurrency == toCurrency {
		return amount, nil
	}
	// Convert to USD first, then to target
	fromRate, ok := ExchangeRates[fromCurrency]
	if !ok {
		return 0, fmt.Errorf("unknown source currency code: %s", fromCurrency)
	}
	toRate, ok := ExchangeRates[toCurrency]
	if !ok {
		return 0, fmt.Errorf("unknown target currency code: %s", toCurrency)
	}
	amountUSD := amount / fromRate
	return amountUSD * toRate, nil
}

// FormatSymbol returns the currency symbol for display.
func FormatSymbol(code Code) string {
	switch code {
	case USD:
		return "$"
	case EUR:
		return "€"
	case GBP:
		return "£"
	case JPY:
		return "¥"
	case CAD:
		return "C$"
	case AUD:
		return "A$"
	case CNY:
		return "¥"
	case INR:
		return "₹"
	default:
		return string(code)
	}
}

// Format returns a human-readable formatted price.
// For JPY and other zero-decimal currencies, precision is 0; otherwise 2.
func Format(amount float64, code Code) string {
	symbol := FormatSymbol(code)
	precision := 2
	if code == JPY {
		precision = 0
	}
	format := fmt.Sprintf("%%.%df", precision)
	return symbol + fmt.Sprintf(format, amount)
}

// IsValidCode returns true if the code is a recognized currency.
func IsValidCode(code Code) bool {
	_, ok := ExchangeRates[code]
	return ok
}

// AllCodes returns a list of all supported currency codes.
func AllCodes() []Code {
	codes := make([]Code, 0, len(ExchangeRates))
	for code := range ExchangeRates {
		codes = append(codes, code)
	}
	return codes
}
