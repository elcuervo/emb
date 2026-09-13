package hfhub

import "testing"

// TestExtraModelFilesIncludesPreprocessorConfig guards the image-embedding
// autoconfiguration contract: DownloadModel must fetch preprocessor_config.json
// so the registry can read rescale/mean/std/crop/resample/size from it.
func TestExtraModelFilesIncludesPreprocessorConfig(t *testing.T) {
	want := map[string]bool{
		"tokenizer.json":           false,
		"config.json":              false,
		"tokenizer_config.json":    false,
		"special_tokens_map.json":  false,
		"preprocessor_config.json": false,
	}
	for _, f := range ExtraModelFiles {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("ExtraModelFiles is missing %q", name)
		}
	}
}
