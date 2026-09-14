package embverify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Corpus is the configurable retrieval input for a verifier: the documents to
// index and the queries to run against them. It is read from a JSON file so a
// caller can verify retrieval against their own data instead of the built-in
// set.
type Corpus struct {
	Documents []string `json:"documents"`
	Queries   []string `json:"queries"`
}

// LoadCorpus reads a JSON corpus from path and validates that it carries at
// least one document and one query. An unreadable, malformed, or empty corpus
// is a named error, never a silent fallback to the built-in set.
func LoadCorpus(path string) (Corpus, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, fmt.Errorf("embverify: read corpus: %w", err)
	}
	var c Corpus
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Corpus{}, fmt.Errorf("embverify: parse corpus %s: %w", path, err)
	}
	if len(c.Documents) == 0 {
		return Corpus{}, fmt.Errorf("embverify: corpus %s has no documents", path)
	}
	if len(c.Queries) == 0 {
		return Corpus{}, fmt.Errorf("embverify: corpus %s has no queries", path)
	}
	return c, nil
}
