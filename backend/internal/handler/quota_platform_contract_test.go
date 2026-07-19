package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageRecordInputsCarryQuotaPlatform(t *testing.T) {
	files := []string{
		"gateway_handler.go",
		"gateway_handler_chat_completions.go",
		"gateway_handler_responses.go",
		"gemini_v1beta_handler.go",
		"openai_alpha_search.go",
		"openai_chat_completions.go",
		"openai_embeddings.go",
		"openai_gateway_handler.go",
		"openai_images.go",
	}

	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
			require.NoError(t, err)

			var missing []token.Position
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok || !isUsageRecordInputLiteral(literal.Type) {
					return true
				}
				if !quotaCompositeLiteralHasKey(literal, "QuotaPlatform") {
					missing = append(missing, fset.Position(literal.Lbrace))
				}
				return true
			})

			require.Empty(t, missing, "usage post-billing must receive request-time QuotaPlatform")
		})
	}
}

func TestBillingEligibilityCallsCarryQuotaPlatform(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	var missing []token.Position
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, parseErr)
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isBillingEligibilityCall(call.Fun) {
				return true
			}
			if len(call.Args) < 6 {
				missing = append(missing, fset.Position(call.Lparen))
			}
			return true
		})
	}

	require.Empty(t, missing, "billing preflight must receive request-time QuotaPlatform")
}

func isUsageRecordInputLiteral(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "service" {
		return false
	}
	switch selector.Sel.Name {
	case "RecordUsageInput", "RecordUsageLongContextInput", "OpenAIRecordUsageInput":
		return true
	default:
		return false
	}
}

func isBillingEligibilityCall(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "CheckBillingEligibility"
}

func quotaCompositeLiteralHasKey(literal *ast.CompositeLit, key string) bool {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := pair.Key.(*ast.Ident)
		if ok && ident.Name == key {
			return true
		}
	}
	return false
}
