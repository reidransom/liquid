package filters

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/reidransom/liquid/expressions"
)

// TestMoneyFilters covers the bug's acceptance matrix and edge cases using the
// default options ($, USD, no thousands separator, input in cents).
func TestMoneyFilters(t *testing.T) {
	cfg := expressions.NewConfig()
	AddMoneyFilters(&cfg, DefaultMoneyOptions())
	ctx := expressions.NewContext(map[string]any{}, cfg)

	tests := []struct {
		in       string
		expected any
	}{
		// bug acceptance table
		{`100000 | money`, "$1000.00"},
		{`100000 | money_with_currency`, "$1000.00 USD"},
		{`100000 | money_without_currency`, "1000.00"},
		{`100000 | money_without_trailing_zeros`, "$1000"},
		{`150000 | money_without_trailing_zeros`, "$1500"},
		{`150050 | money_without_trailing_zeros`, "$1500.50"},
		{`150050 | money`, "$1500.50"},
		{`150050 | money_with_currency`, "$1500.50 USD"},
		{`150050 | money_without_currency`, "1500.50"},

		// edges
		{`0 | money`, "$0.00"},
		{`0 | money_without_trailing_zeros`, "$0"},
		{`1 | money`, "$0.01"},
		{`99 | money`, "$0.99"},
		{`100 | money_without_trailing_zeros`, "$1"},
		{`-1000 | money`, "-$10.00"},
		{`-1000 | money_without_trailing_zeros`, "-$10"},
		{`-1000 | money_with_currency`, "-$10.00 USD"},
		{`-1000 | money_without_currency`, "-10.00"},

		// coercion (Q1: unparseable string -> $0.00)
		{`"100000" | money`, "$1000.00"},
		{`"abc" | money`, "$0.00"},

		// nil / empty -> "" (absent value)
		{`nil | money`, ""},
		{`nil | money_without_trailing_zeros`, ""},
		{`nil | money_with_currency`, ""},
		{`"" | money`, ""},
		{`"" | money_without_trailing_zeros`, ""},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%02d", i+1), func(t *testing.T) {
			actual, err := expressions.EvaluateString(test.in, ctx)
			require.NoErrorf(t, err, test.in)
			require.Equalf(t, test.expected, actual, test.in)
		})
	}
}

// TestMoneyFiltersCustomOptions proves the options actually flow through the
// registration-time closures: a custom symbol, currency code, and thousands
// separator affect all four filters.
func TestMoneyFiltersCustomOptions(t *testing.T) {
	cfg := expressions.NewConfig()
	AddMoneyFilters(&cfg, MoneyOptions{
		Symbol:             "€",
		CurrencyCode:       "EUR",
		ThousandsSeparator: ",",
		InputIsCents:       true,
	})
	ctx := expressions.NewContext(map[string]any{}, cfg)

	tests := []struct {
		in       string
		expected any
	}{
		{`100000 | money`, "€1,000.00"},
		{`100000 | money_with_currency`, "€1,000.00 EUR"},
		{`100000 | money_without_currency`, "1,000.00"},
		{`100000 | money_without_trailing_zeros`, "€1,000"},
		{`150050 | money_without_trailing_zeros`, "€1,500.50"},
		{`-100000 | money`, "-€1,000.00"},
		{`-100000 | money_without_trailing_zeros`, "-€1,000"},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%02d", i+1), func(t *testing.T) {
			actual, err := expressions.EvaluateString(test.in, ctx)
			require.NoErrorf(t, err, test.in)
			require.Equalf(t, test.expected, actual, test.in)
		})
	}
}

// TestMoneyFiltersDollarMode covers InputIsCents=false (the doc-example mode),
// where the input is a dollar amount and is not divided by 100.
func TestMoneyFiltersDollarMode(t *testing.T) {
	cfg := expressions.NewConfig()
	AddMoneyFilters(&cfg, MoneyOptions{
		Symbol:       "$",
		CurrencyCode: "USD",
		InputIsCents: false,
	})
	ctx := expressions.NewContext(map[string]any{}, cfg)

	tests := []struct {
		in       string
		expected any
	}{
		{`1000 | money`, "$1000.00"}, // no /100
		{`1000 | money_without_trailing_zeros`, "$1000"},
		{`10.50 | money`, "$10.50"},
		{`10.50 | money_without_trailing_zeros`, "$10.50"},
		{`10.00 | money_without_trailing_zeros`, "$10"},
		{`10 | money_with_currency`, "$10.00 USD"},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%02d", i+1), func(t *testing.T) {
			actual, err := expressions.EvaluateString(test.in, ctx)
			require.NoErrorf(t, err, test.in)
			require.Equalf(t, test.expected, actual, test.in)
		})
	}
}

// TestMoneyFiltersZeroValueDefaults proves a zero-value MoneyOptions falls back
// to DefaultMoneyOptions (Q2).
func TestMoneyFiltersZeroValueDefaults(t *testing.T) {
	cfg := expressions.NewConfig()
	AddMoneyFilters(&cfg, MoneyOptions{})
	ctx := expressions.NewContext(map[string]any{}, cfg)

	actual, err := expressions.EvaluateString(`100000 | money_with_currency`, ctx)
	require.NoError(t, err)
	require.Equal(t, "$1000.00 USD", actual)
}
