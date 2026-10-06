package emoji

import (
	"strings"
	"testing"
)

// asset is the committed vocabulary the DNS zone answers from.
const asset = "../../dns/emoji-vocab.json"

func loadAsset(t *testing.T) Vocab {
	t.Helper()
	v, err := Load(asset)
	if err != nil {
		t.Fatalf("Load(%s): %v", asset, err)
	}
	return v
}

func fixture(entries string) []byte {
	return []byte(`{"source":"test","license":"test","entries":[` + entries + `]}`)
}

func TestLoadRealVocabulary(t *testing.T) {
	v := loadAsset(t)
	// Load validates the invariants: no duplicate slug, no duplicate glyph, no
	// empty glyph, slug, or description. Getting here is the check.
	if v.Len() < 1000 {
		t.Fatalf("vocabulary holds %d entries, want the full annotated set", v.Len())
	}
	if v.Source == "" || v.License == "" {
		t.Fatalf("vocabulary carries no provenance: source=%q license=%q", v.Source, v.License)
	}
	for _, e := range v.Entries {
		if strings.ContainsAny(e.Slug, "+-") {
			t.Fatalf("slug %q carries a query operator", e.Slug)
		}
		if !strings.HasPrefix(e.Description, e.Name) {
			t.Fatalf("entry %q describes itself as %q, which does not open with its name", e.Slug, e.Description)
		}
	}
}

func TestLoadRejectsAmbiguousVocabulary(t *testing.T) {
	cases := map[string]string{
		"duplicate slug":         `{"glyph":"👑","slug":"crown","name":"crown","description":"crown"},{"glyph":"💎","slug":"crown","name":"gem","description":"gem"}`,
		"duplicate glyph":        `{"glyph":"👑","slug":"crown","name":"crown","description":"crown"},{"glyph":"👑","slug":"crowns","name":"crowns","description":"crowns"}`,
		"missing glyph":          `{"glyph":"","slug":"crown","name":"crown","description":"crown"}`,
		"missing slug":           `{"glyph":"👑","slug":"","name":"crown","description":"crown"}`,
		"missing description":    `{"glyph":"👑","slug":"crown","name":"crown","description":"  "}`,
		"unparseable document":   `not json`,
		"provenance is optional": ``, // an empty vocabulary is legal, if useless
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			raw := entries
			if raw != "not json" {
				raw = string(fixture(entries))
			}
			v, err := LoadBytes([]byte(raw))
			if strings.Contains(name, "optional") {
				if err != nil {
					t.Fatalf("LoadBytes: %v", err)
				}
				if v.Len() != 0 {
					t.Fatalf("expected an empty vocabulary, got %d entries", v.Len())
				}
				return
			}
			if err == nil {
				t.Fatalf("LoadBytes accepted %s", name)
			}
		})
	}
}

func TestLookupFindsSlugAndGlyph(t *testing.T) {
	v := loadAsset(t)
	bySlug, ok := v.Lookup("crown")
	if !ok {
		t.Fatal("crown is not in the vocabulary")
	}
	byGlyph, ok := v.Lookup("👑")
	if !ok {
		t.Fatal("👑 is not in the vocabulary")
	}
	if bySlug != byGlyph {
		t.Fatalf("👑 resolved to %q and crown to %q", byGlyph.Slug, bySlug.Slug)
	}
	if _, ok := v.Lookup("definitely not an emoji"); ok {
		t.Fatal("lookup matched a word that names nothing")
	}
}
