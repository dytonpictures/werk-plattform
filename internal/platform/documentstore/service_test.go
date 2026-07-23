package documentstore

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeListQuery(t *testing.T) {
	query, err := NormalizeListQuery(ListQuery{Search: "  Vertrag  "})
	if err != nil || query.Limit != DefaultLimit || query.Search != "Vertrag" {
		t.Fatalf("normalized query = %#v, err = %v", query, err)
	}
	validCursor := Cursor{UpdatedAt: time.Now().UTC(), ID: "0196f000-0000-7000-8000-000000000901"}
	if _, err := NormalizeListQuery(ListQuery{
		Limit: MaximumLimit, Status: "active", Classification: "confidential",
		AccessReason: "shared-directly-with-me", Cursor: &validCursor,
	}); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	for name, candidate := range map[string]ListQuery{
		"limit":          {Limit: MaximumLimit + 1},
		"search":         {Search: strings.Repeat("a", maximumSearchSize+1)},
		"status":         {Status: "deleted"},
		"classification": {Classification: "public"},
		"access":         {AccessReason: "tenant-wide"},
		"cursor":         {Cursor: &Cursor{UpdatedAt: time.Now().UTC(), ID: "not-a-uuid"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeListQuery(candidate); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("error = %v, want invalid query", err)
			}
		})
	}
}
