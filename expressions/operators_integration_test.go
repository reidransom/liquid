package expressions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogicalOperatorsIntegration tests that both word-based (and/or) and
// symbol-based (&&/||) logical operators work correctly, addressing the issue
// where && and || would cause a syntax error panic.
func TestLogicalOperatorsIntegration(t *testing.T) {
	cfg := NewConfig()
	bindings := map[string]any{
		"lang": "en",
		"name": "faq.md",
		"pages": []map[string]any{
			{"lang": "en", "name": "faq.md"},
			{"lang": "en", "name": "about.md"},
			{"lang": "fr", "name": "faq.md"},
		},
	}
	ctx := NewContext(bindings, cfg)

	tests := []struct {
		name     string
		expr     string
		expected any
	}{
		// Word-based operators (existing functionality)
		{
			name:     "and operator with simple booleans",
			expr:     "true and false",
			expected: false,
		},
		{
			name:     "or operator with simple booleans",
			expr:     "false or true",
			expected: true,
		},
		{
			name:     "and operator with comparisons",
			expr:     `lang == "en" and name == "faq.md"`,
			expected: true,
		},
		{
			name:     "or operator with comparisons",
			expr:     `lang == "fr" or name == "faq.md"`,
			expected: true,
		},

		// Symbol-based operators (new functionality)
		{
			name:     "&& operator with simple booleans",
			expr:     "true && false",
			expected: false,
		},
		{
			name:     "|| operator with simple booleans",
			expr:     "false || true",
			expected: true,
		},
		{
			name:     "&& operator with comparisons",
			expr:     `lang == "en" && name == "faq.md"`,
			expected: true,
		},
		{
			name:     "|| operator with comparisons",
			expr:     `lang == "fr" || name == "faq.md"`,
			expected: true,
		},

		// Mixed operators
		{
			name:     "chained && operators",
			expr:     `lang == "en" && name == "faq.md" && true`,
			expected: true,
		},
		{
			name:     "chained || operators",
			expr:     `false || false || name == "faq.md"`,
			expected: true,
		},

		// Complex expressions like those used in where_exp
		{
			name:     "complex && expression (where_exp style)",
			expr:     `lang == "en" && name == "faq.md"`,
			expected: true,
		},
		{
			name:     "complex || expression",
			expr:     `name == "missing.md" || name == "faq.md"`,
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := EvaluateString(test.expr, ctx)
			require.NoError(t, err, "Expression should not cause a syntax error: %s", test.expr)
			require.Equal(t, test.expected, result, "Expression: %s", test.expr)
		})
	}
}

// TestOperatorPrecedence verifies that && and || have the same precedence as and/or
func TestOperatorPrecedence(t *testing.T) {
	cfg := NewConfig()
	ctx := NewContext(map[string]any{}, cfg)

	tests := []struct {
		expr1    string // using word operators
		expr2    string // using symbol operators
		expected any
	}{
		{
			expr1:    "true and false or true",
			expr2:    "true && false || true",
			expected: true,
		},
		{
			expr1:    "false or true and false",
			expr2:    "false || true && false",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.expr1, func(t *testing.T) {
			result1, err := EvaluateString(test.expr1, ctx)
			require.NoError(t, err)
			require.Equal(t, test.expected, result1)

			result2, err := EvaluateString(test.expr2, ctx)
			require.NoError(t, err)
			require.Equal(t, test.expected, result2)

			// Both forms should produce the same result
			require.Equal(t, result1, result2, "Word and symbol operators should be equivalent")
		})
	}
}
