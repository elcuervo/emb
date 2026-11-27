package contract

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"
)

func TestDocsCoverAndSort(t *testing.T) {
	if len(Docs) != 22 {
		t.Fatalf("len(Docs) = %d, want 22", len(Docs))
	}
	if !sort.SliceIsSorted(Docs, func(i, j int) bool { return Docs[i].Name < Docs[j].Name }) {
		t.Fatal("Docs is not sorted by Name")
	}
	seen := make(map[string]bool, len(Docs))
	for _, d := range Docs {
		if seen[d.Name] {
			t.Fatalf("duplicate command %q", d.Name)
		}
		seen[d.Name] = true
	}
}

func TestByNameRoundTrips(t *testing.T) {
	for _, d := range Docs {
		got, ok := ByName(d.Name)
		if !ok {
			t.Fatalf("ByName(%q) not found", d.Name)
		}
		if got.Name != d.Name {
			t.Fatalf("ByName(%q).Name = %q", d.Name, got.Name)
		}
	}
	if _, ok := ByName("NOPE"); ok {
		t.Fatal("ByName found an undeclared command")
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	first, err := Render()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Render is not byte-identical across calls")
	}
	if !bytes.HasSuffix(first, []byte("\n")) {
		t.Fatal("Render has no trailing newline")
	}
	var envelope struct {
		Contract int   `json:"contract"`
		Commands []Doc `json:"commands"`
	}
	if err := json.Unmarshal(first, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Contract != 1 {
		t.Fatalf("contract = %d, want 1", envelope.Contract)
	}
	if len(envelope.Commands) != 22 {
		t.Fatalf("rendered commands = %d, want 22", len(envelope.Commands))
	}
}

type schemaBranch struct {
	Type  string `json:"type"`
	Items struct {
		OneOf []struct {
			Type string `json:"type"`
		} `json:"oneOf"`
	} `json:"items"`
}

type replySchema struct {
	DollarSchema string         `json:"$schema"`
	Type         string         `json:"type"`
	OneOf        []schemaBranch `json:"oneOf"`
	Items        struct {
		OneOf []struct {
			Type string `json:"type"`
		} `json:"oneOf"`
	} `json:"items"`
}

func TestImageReplySchemas(t *testing.T) {
	parse := func(name string) replySchema {
		d, ok := ByName(name)
		if !ok {
			t.Fatalf("%s not declared", name)
		}
		var schema replySchema
		if err := json.Unmarshal(d.ReplySchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", name, err)
		}
		if schema.DollarSchema != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s $schema = %q", name, schema.DollarSchema)
		}
		if d.WireNotes == "" {
			t.Fatalf("%s has no wire_notes", name)
		}
		return schema
	}
	hasNull := func(types []struct {
		Type string `json:"type"`
	}) bool {
		for _, b := range types {
			if b.Type == "null" {
				return true
			}
		}
		return false
	}

	// One image can be a failed (null) slot, a single keyed map, or an array of
	// maps: writeIMGResult collapses only a one-image request that succeeded.
	img := parse("EMB.IMG")
	branches := make(map[string]schemaBranch, len(img.OneOf))
	for _, b := range img.OneOf {
		branches[b.Type] = b
	}
	if len(branches) != 3 || branches["object"].Type == "" || branches["array"].Type == "" {
		t.Fatalf("EMB.IMG oneOf = %#v, want null, object and array branches", img.OneOf)
	}
	if !hasNull(branches["array"].Items.OneOf) {
		t.Fatalf("EMB.IMG array branch has no nullable slot")
	}

	// EMB.IMGMULTI always replies one slot per pair, so a bare object is invalid.
	imgMulti := parse("EMB.IMGMULTI")
	if imgMulti.Type != "array" {
		t.Fatalf("EMB.IMGMULTI type = %q, want array", imgMulti.Type)
	}
	if len(imgMulti.OneOf) != 0 {
		t.Fatalf("EMB.IMGMULTI has %d top-level oneOf branches, want none", len(imgMulti.OneOf))
	}
	if !hasNull(imgMulti.Items.OneOf) {
		t.Fatalf("EMB.IMGMULTI array items have no nullable slot")
	}

	for _, d := range Docs {
		if d.Name != "EMB.IMG" && d.Name != "EMB.IMGMULTI" && len(d.ReplySchema) != 0 {
			t.Fatalf("%s carries an unexpected reply_schema", d.Name)
		}
	}
}
