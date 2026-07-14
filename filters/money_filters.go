package filters

import (
	"strconv"
	"strings"
)

// MoneyOptions configures the Shopify commerce "money" filter family.
//
// Shopify's money filters format a price. In real Shopify themes the price
// (e.g. product.price) is an integer number of cents; the store's currency
// format strings determine the symbol, code, and separators. Shopify's public
// filter docs (https://shopify.dev/docs/api/liquid/filters#money) illustrate
// the filters with a synthetic dollar-amount input, but the production
// semantics are cents-based. This implementation therefore treats numeric
// input as cents and divides by 100 by default; set InputIsCents to false to
// pass dollar amounts directly (the doc-example mode). See
// _issues/BUG-money-filters.md and _issues/PLAN-money-filters.md.
//
// The defaults ($, USD, no thousands separator) match Shopify's default
// "HTML without currency" format ("${{amount}}"), which produces no thousands
// grouping. Set ThousandsSeparator (e.g. ",") to enable grouping.
type MoneyOptions struct {
	// Symbol is prepended to the amount, e.g. "$". Default "$".
	Symbol string
	// CurrencyCode is appended after a space by money_with_currency, e.g. "USD". Default "USD".
	CurrencyCode string
	// ThousandsSeparator, if non-empty, groups the integer part in threes.
	// Default "" (no grouping, matching Shopify's default "${{amount}}" format).
	ThousandsSeparator string
	// InputIsCents true (the default) divides numeric input by 100.
	// Set false to treat the input as a dollar amount (the doc-example mode).
	InputIsCents bool
}

// DefaultMoneyOptions returns the default money filter configuration.
func DefaultMoneyOptions() MoneyOptions {
	return MoneyOptions{
		Symbol:       "$",
		CurrencyCode: "USD",
		InputIsCents: true,
	}
}

// AddMoneyFilters registers the Shopify "money" filter family: money,
// money_with_currency, money_without_currency, and
// money_without_trailing_zeros. The filters close over opts so the format is
// captured at registration time — Liquid filters receive only their arguments
// at call time, not the engine Config, so the format cannot be read from Config
// during rendering.
//
// A zero-value opts is replaced with DefaultMoneyOptions().
//
// Input is coerced to a number via toFloat64 (numeric strings are parsed;
// an unparseable string yields 0, producing "$0.00"). A nil or empty-string
// input renders as "" (no value to format). Negative amounts place the sign
// before the symbol, e.g. "-$10.00".
func AddMoneyFilters(fd FilterDictionary, opts MoneyOptions) {
	if opts == (MoneyOptions{}) {
		opts = DefaultMoneyOptions()
	}

	fd.AddFilter("money", func(v any) string {
		return applyMoney(v, opts, func(d float64) string { return moneyString(d, opts, true) })
	})
	fd.AddFilter("money_with_currency", func(v any) string {
		return applyMoney(v, opts, func(d float64) string {
			return moneyString(d, opts, true) + " " + opts.CurrencyCode
		})
	})
	fd.AddFilter("money_without_currency", func(v any) string {
		return applyMoney(v, opts, func(d float64) string { return moneyString(d, opts, false) })
	})
	fd.AddFilter("money_without_trailing_zeros", func(v any) string {
		return applyMoney(v, opts, func(d float64) string { return moneyWithoutTrailingZeros(d, opts) })
	})
}

// applyMoney handles input normalization shared by all four money filters:
// nil and empty-string render as ""; []byte is presented as a string (matching
// the engine's documented treatment of []byte — see README "Value Types");
// everything else is coerced to dollars and handed to format.
func applyMoney(v any, opts MoneyOptions, format func(dollars float64) string) string {
	if v == nil {
		return ""
	}
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if s, ok := v.(string); ok && s == "" {
		return ""
	}
	return format(moneyToDollars(v, opts))
}

// moneyToDollars coerces a filter input to a dollar amount. Numeric strings are
// parsed via toFloat64 (which returns 0 on parse failure). The value is
// divided by 100 when InputIsCents is true.
func moneyToDollars(v any, opts MoneyOptions) float64 {
	n := toFloat64(v)
	if opts.InputIsCents {
		n /= 100
	}
	return n
}

// moneyString formats dollars as a price, prefixed by the currency symbol when
// withSymbol is true. The sign ("-") precedes the symbol for negative amounts.
func moneyString(dollars float64, opts MoneyOptions, withSymbol bool) string {
	sign, body := formatMoneyAmount(dollars, opts)
	if withSymbol {
		return sign + opts.Symbol + body
	}
	return sign + body
}

// moneyWithoutTrailingZeros formats like money, but drops the decimal separator
// and the two trailing zeros when the amount has no fractional cents. If the
// amount has a non-zero fractional part the output is identical to money.
// (See https://shopify.dev/docs/api/liquid/filters/money_without_trailing_zeros.)
//
// The check is performed on the formatted string rather than on the float, so
// it is robust against floating-point representation error in the cents/100
// division.
func moneyWithoutTrailingZeros(dollars float64, opts MoneyOptions) string {
	sign, body := formatMoneyAmount(dollars, opts)
	body = strings.TrimSuffix(body, ".00")
	return sign + opts.Symbol + body
}

// formatMoneyAmount formats dollars with two decimal places and optional
// thousands grouping. It returns the sign ("" or "-") and the unsigned body so
// callers can place the sign before the currency symbol.
func formatMoneyAmount(dollars float64, opts MoneyOptions) (sign, body string) {
	// Normalize negative zero so FormatFloat doesn't emit "-0.00".
	if dollars == 0 {
		dollars = 0
	}
	if dollars < 0 {
		sign = "-"
		dollars = -dollars
	}
	body = strconv.FormatFloat(dollars, 'f', 2, 64)
	if opts.ThousandsSeparator != "" {
		body = groupThousands(body, opts.ThousandsSeparator)
	}
	return sign, body
}

// groupThousands inserts sep between each group of three integer digits in s,
// which is expected to be a fixed-point decimal string such as "1000.00".
// Amounts with fewer than four integer digits are returned unchanged.
func groupThousands(s, sep string) string {
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	if len(intPart) <= 3 {
		return s
	}
	var b strings.Builder
	first := len(intPart) % 3
	if first > 0 {
		b.WriteString(intPart[:first])
		b.WriteString(sep)
	}
	for i := first; i < len(intPart); i += 3 {
		b.WriteString(intPart[i : i+3])
		if i+3 < len(intPart) {
			b.WriteString(sep)
		}
	}
	b.WriteString(frac)
	return b.String()
}
