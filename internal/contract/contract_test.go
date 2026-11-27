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

func TestImageReplySchemas(t *testing.T) {
	for _, name := range []string{"EMB.IMG", "EMB.IMGMULTI"} {
		d, ok := ByName(name)
		if !ok {
			t.Fatalf("%s not declared", name)
		}
		var schema struct {
			DollarSchema string `json:"$schema"`
			OneOf        []struct {
				Type  string `json:"type"`
				Items struct {
					OneOf []struct {
						Type string `json:"type"`
					} `json:"oneOf"`
				} `json:"items"`
			} `json:"oneOf"`
		}
		if err := json.Unmarshal(d.ReplySchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", name, err)
		}
		if schema.DollarSchema != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s $schema = %q", name, schema.DollarSchema)
		}
		if len(schema.OneOf) != 2 {
			t.Fatalf("%s oneOf has %d branches, want 2", name, len(schema.OneOf))
		}
		if schema.OneOf[0].Type != "object" {
			t.Fatalf("%s oneOf[0].type = %q, want object", name, schema.OneOf[0].Type)
		}
		if schema.OneOf[1].Type != "array" {
			t.Fatalf("%s oneOf[1].type = %q, want array", name, schema.OneOf[1].Type)
		}
		nullable := false
		for _, item := range schema.OneOf[1].Items.OneOf {
			if item.Type == "null" {
				nullable = true
			}
		}
		if !nullable {
			t.Fatalf("%s array branch has no nullable slot", name)
		}
		if d.WireNotes == "" {
			t.Fatalf("%s has no wire_notes", name)
		}
	}
	for _, d := range Docs {
		if d.Name != "EMB.IMG" && d.Name != "EMB.IMGMULTI" && len(d.ReplySchema) != 0 {
			t.Fatalf("%s carries an unexpected reply_schema", d.Name)
		}
	}
}
