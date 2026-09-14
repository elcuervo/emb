package embverify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCorpus(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	c, err := LoadCorpus(write("ok.json", `{"documents":["a","b"],"queries":["q"]}`))
	if err != nil {
		t.Fatalf("valid corpus: %v", err)
	}
	if len(c.Documents) != 2 || len(c.Queries) != 1 {
		t.Fatalf("corpus = %+v, want 2 documents and 1 query", c)
	}

	for name, body := range map[string]string{
		"empty":      `{}`,
		"no-queries": `{"documents":["a"]}`,
		"no-docs":    `{"queries":["q"]}`,
		"malformed":  `{`,
		"unknown":    `{"documents":["a"],"queries":["q"],"extra":true}`,
	} {
		if _, err := LoadCorpus(write(name+".json", body)); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}

	if _, err := LoadCorpus(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file: expected an error")
	}
}
