package emoji

import (
	"errors"
	"strings"
	"testing"
)

func TestParseReadsSentencesAndTerms(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   []Term
	}{
		{
			name:   "labels are the words of one sentence",
			labels: []string{"my", "mom", "is", "in", "the", "hospital"},
			want:   []Term{{Text: "my mom is in the hospital"}},
		},
		{
			name:   "a composition separates terms",
			labels: []string{"shark-fish+bird"},
			want:   []Term{{Text: "shark"}, {Subtract: true, Text: "fish"}, {Text: "bird"}},
		},
		{
			name:   "operators split inside a label and dots join inside a term",
			labels: []string{"crown-man", "woman"},
			want:   []Term{{Text: "crown"}, {Subtract: true, Text: "man woman"}},
		},
		{
			name:   "case does not matter",
			labels: []string{"CROWN-Man"},
			want:   []Term{{Text: "crown"}, {Subtract: true, Text: "man"}},
		},
		{
			name:   "a slug keeps its underscores",
			labels: []string{"loudly_crying_face"},
			want:   []Term{{Text: "loudly_crying_face"}},
		},
		{
			name:   "a glyph travels as written",
			labels: []string{"🦈"},
			want:   []Term{{Text: "🦈"}},
		},
		{
			name:   "a trailing dot is not a word",
			labels: []string{"lost", "job"},
			want:   []Term{{Text: "lost job"}},
		},
		{
			name:   "repeated operators do not invent empty terms",
			labels: []string{"shark--fish"},
			want:   []Term{{Text: "shark"}, {Subtract: true, Text: "fish"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.labels)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.labels, err)
			}
			if len(got.Terms) != len(tc.want) {
				t.Fatalf("Parse(%q) gave %d terms, want %d: %+v", tc.labels, len(got.Terms), len(tc.want), got.Terms)
			}
			for i, term := range got.Terms {
				if term != tc.want[i] {
					t.Errorf("term %d is %+v, want %+v", i, term, tc.want[i])
				}
			}
		})
	}
}

func TestParseKnowsWhatASentenceIs(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   bool
	}{
		{"a sentence", []string{"my", "mom", "is", "in", "the", "hospital"}, true},
		{"a slug is one word", []string{"loudly_crying_face"}, false},
		{"one word", []string{"shark"}, false},
		{"one glyph", []string{"🦈"}, false},
		{"a composition", []string{"🦈-🐟+🐦"}, false},
		{"a composition of sentences", []string{"my", "mom", "-your", "dad"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.labels)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.labels, err)
			}
			if got.Sentence != tc.want {
				t.Fatalf("Parse(%q).Sentence is %v, want %v", tc.labels, got.Sentence, tc.want)
			}
		})
	}
}

func TestSpellingAnEntryIsStillOneWord(t *testing.T) {
	v := loadAsset(t)
	// `shark` names an entry whose description is several words long. The
	// query wrote one word, so it is not a sentence — the distinction is made
	// before the description is substituted.
	query, err := v.Query([]string{"shark"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if query.Sentence {
		t.Fatalf("naming an entry became a sentence: %+v", query.Terms)
	}
	if len(strings.Fields(query.Terms[0].Text)) < 2 {
		t.Fatalf("the fixture entry describes itself in one word (%q), so this test proves nothing", query.Terms[0].Text)
	}
}

func TestParseRefusesWhatItCannotAnswer(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   error
	}{
		{"no labels at all", nil, ErrEmpty},
		{"one empty label", []string{""}, ErrEmpty},
		{"only separators", []string{"-+-"}, ErrNoTerms},
		{"opens with a subtraction", []string{"-crown"}, ErrNoTerms},
		{"a label past the transport's limit", []string{strings.Repeat("a", MaxLabelBytes+1)}, ErrTooLong},
		{"a name past the transport's limit", []string{strings.Repeat("aaaaaaaa", 32)}, ErrTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.labels)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse(%q) gave %v, want %v", tc.labels, err, tc.want)
			}
		})
	}
}

func TestSpellNamesVocabularyWords(t *testing.T) {
	v := loadAsset(t)
	crown, ok := v.Lookup("crown")
	if !ok {
		t.Fatal("crown is not in the vocabulary")
	}

	cases := []struct {
		name   string
		labels []string
		want   string
	}{
		{"a glyph becomes its description", []string{"👑"}, crown.Description},
		{"its slug becomes the same description", []string{"crown"}, crown.Description},
		{"a word inside a sentence is spelled out", []string{"the", "👑", "is", "heavy"}, "the " + crown.Description + " is heavy"},
		{"a word that names nothing is left alone", []string{"my", "mom", "is", "sick"}, "my mom is sick"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := v.Query(tc.labels)
			if err != nil {
				t.Fatalf("Query(%q): %v", tc.labels, err)
			}
			if len(got.Terms) != 1 {
				t.Fatalf("expected one term, got %+v", got.Terms)
			}
			if got.Terms[0].Text != tc.want {
				t.Fatalf("text is %q, want %q", got.Terms[0].Text, tc.want)
			}
		})
	}
}

func TestGlyphAndSlugProduceOneQuery(t *testing.T) {
	v := loadAsset(t)
	glyph, err := v.Query([]string{"🦈"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	slug, err := v.Query([]string{"shark"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if glyph.Terms[0].Text != slug.Terms[0].Text {
		t.Fatalf("🦈 spelled %q and shark spelled %q", glyph.Terms[0].Text, slug.Terms[0].Text)
	}
}

func TestParseReadsAConjunction(t *testing.T) {
	got, err := Parse([]string{"🍕*🦅"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Term{{Text: "🍕"}, {Conjoin: true, Text: "🦅"}}
	if len(got.Terms) != len(want) {
		t.Fatalf("Parse gave %d terms, want %d: %+v", len(got.Terms), len(want), got.Terms)
	}
	for i, term := range got.Terms {
		if term != want[i] {
			t.Errorf("term %d is %+v, want %+v", i, term, want[i])
		}
	}
	if !got.Conjunction {
		t.Fatal("a name joined by `*` is not marked a conjunction")
	}
	if got.Sentence {
		t.Fatal("a conjunction is not a sentence")
	}
}

func TestParseReadsAConjunctionOfMoreThanTwoTerms(t *testing.T) {
	got, err := Parse([]string{"🍕*🦅*🍔"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Terms) != 3 {
		t.Fatalf("Parse gave %d terms, want 3: %+v", len(got.Terms), got.Terms)
	}
	for i, term := range got.Terms[1:] {
		if !term.Conjoin {
			t.Errorf("term %d is not conjoined: %+v", i+1, term)
		}
	}
	if !got.Conjunction {
		t.Fatal("a three-term conjunction is not marked a conjunction")
	}
}

func TestParseRefusesMixedOperators(t *testing.T) {
	for _, name := range []string{"🍕*🦅+🍔", "🍕-🦅*🍔", "🍕+🦅*🍔", "🍕*🦅-🍔"} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]string{name}); !errors.Is(err, ErrMixedOperators) {
				t.Fatalf("Parse(%q) gave %v, want %v", name, err, ErrMixedOperators)
			}
		})
	}
}

func TestParseRefusesANameWithNoTerm(t *testing.T) {
	for _, name := range []string{"*", "***", "*-*"} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]string{name}); !errors.Is(err, ErrNoTerms) {
				t.Fatalf("Parse(%q) gave %v, want %v", name, err, ErrNoTerms)
			}
		})
	}
}

func TestOneTermIsNotAConjunction(t *testing.T) {
	for _, name := range []string{"🍕", "*🍕", "🦈-🐟+🐦"} {
		got, err := Parse([]string{name})
		if err != nil {
			t.Fatalf("Parse(%q): %v", name, err)
		}
		if got.Conjunction {
			t.Fatalf("Parse(%q) is a conjunction of %d terms", name, len(got.Terms))
		}
	}
}

func TestConjunctionSpellingMatchesTheGlyphForm(t *testing.T) {
	v := loadAsset(t)
	glyph, err := v.Query([]string{"pizza*eagle"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	words, err := v.Query([]string{"🍕*🦅"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !glyph.Conjunction || !words.Conjunction {
		t.Fatalf("a conjunction was spelled into %+v and %+v", glyph, words)
	}
	if len(glyph.Terms) != len(words.Terms) {
		t.Fatalf("the two spellings have %d and %d terms", len(glyph.Terms), len(words.Terms))
	}
	for i := range glyph.Terms {
		if glyph.Terms[i].Text != words.Terms[i].Text {
			t.Fatalf("term %d spelled %q and %q", i, glyph.Terms[i].Text, words.Terms[i].Text)
		}
	}
}

func TestIndicesAreTheEntriesAQueryNames(t *testing.T) {
	v := loadAsset(t)
	query, err := v.Query([]string{"pizza*🦅"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	parsed, err := Parse([]string{"pizza*🦅"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Terms) != 2 {
		t.Fatalf("expected two terms, got %+v", query.Terms)
	}
	named := v.Indices(parsed)
	if len(named) != 2 {
		t.Fatalf("Indices returned %d entries, want 2: %v", len(named), named)
	}
	for _, term := range parsed.Terms {
		entry, ok := v.Lookup(term.Text)
		if !ok {
			t.Fatalf("%q names no entry, so this test proves nothing", term.Text)
		}
		i, ok := v.bySlug[entry.Slug]
		if !ok {
			t.Fatalf("%q is not indexed by slug", entry.Slug)
		}
		if _, ok := named[i]; !ok {
			t.Fatalf("Indices did not include %q", entry.Slug)
		}
	}
}

func TestIndicesOfFreeTextAreEmpty(t *testing.T) {
	v := loadAsset(t)
	parsed, err := Parse([]string{"zzzqqq*xxyy"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if named := v.Indices(parsed); len(named) != 0 {
		t.Fatalf("free text named %d entries: %v", len(named), named)
	}
}
