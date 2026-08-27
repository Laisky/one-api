package middleware

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsModelInList pins the exact matching rule of the token allow-list.
//
// This predicate is the single authority for both enforcement (TokenAuth's 403)
// and discovery (controller.ListModels / RetrieveModel / GetAvailableModelsByToken).
// A change here silently changes who can see and call what, so every property is
// pinned explicitly rather than left to the callers' tests, which would otherwise
// only prove that they call this function.
func TestIsModelInList(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		model  string
		list   string
		expect bool
	}{
		"exact single":                 {model: "gpt-4o", list: "gpt-4o", expect: true},
		"exact among many":             {model: "b", list: "a,b,c", expect: true},
		"absent":                       {model: "d", list: "a,b,c", expect: false},
		"case sensitive":               {model: "gpt-4o", list: "GPT-4O", expect: false},
		"case sensitive inverse":       {model: "GPT-4O", list: "gpt-4o", expect: false},
		"leading space not trimmed":    {model: "b", list: "a, b", expect: false},
		"trailing space not trimmed":   {model: "a", list: "a ,b", expect: false},
		"space matches raw entry":      {model: " b", list: "a, b", expect: true},
		"empty entries preserved":      {model: "b", list: "a,,b", expect: true},
		"no wildcard support":          {model: "gpt-4o", list: "gpt-4*", expect: false},
		"literal star only":            {model: "*", list: "*", expect: true},
		"no prefix matching":           {model: "gpt-4o-mini", list: "gpt-4o", expect: false},
		"not a substring match":        {model: "pt-4", list: "gpt-4o", expect: false},
		"semicolon is not a separator": {model: "b", list: "a;b", expect: false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expect, IsModelInList(tc.model, tc.list))
		})
	}
}
