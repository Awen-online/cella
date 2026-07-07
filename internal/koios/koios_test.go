package koios

import (
	"encoding/json"
	"testing"
)

func TestGovernanceActionTitle(t *testing.T) {
	tests := []struct {
		name string
		meta string
		want string
	}{
		{"cip108 body.title", `{"body":{"title":"Increase treasury","abstract":"x"}}`, "Increase treasury"},
		{"no metadata", ``, ""},
		{"no title field", `{"body":{"abstract":"x"}}`, ""},
		{"malformed json", `not json`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := GovernanceAction{}
			if tt.meta != "" {
				a.MetaJSON = json.RawMessage(tt.meta)
			}
			if got := a.Title(); got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}
