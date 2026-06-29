package registry

import (
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

func TestSelectOutputTensorPrefersRank2(t *testing.T) {
	outputs := map[string]onnx.OutputInfo{
		"last_hidden_state": {Name: "last_hidden_state", Rank: 3, Dim: 768},
		"pooler_output":     {Name: "pooler_output", Rank: 2, Dim: 768},
	}
	name, rank := selectOutputTensor(outputs)
	if name != "pooler_output" {
		t.Fatalf("expected pooler_output, got %s", name)
	}
	if rank != 2 {
		t.Fatalf("expected rank 2, got %d", rank)
	}
}

func TestSelectOutputTensorPicksOnlyAvailable(t *testing.T) {
	outputs := map[string]onnx.OutputInfo{
		"last_hidden_state": {Name: "last_hidden_state", Rank: 3, Dim: 384},
	}
	name, rank := selectOutputTensor(outputs)
	if name != "last_hidden_state" {
		t.Fatalf("expected last_hidden_state, got %s", name)
	}
	if rank != 3 {
		t.Fatalf("expected rank 3, got %d", rank)
	}
}

func TestSelectOutputTensorRank3(t *testing.T) {
	outputs := map[string]onnx.OutputInfo{
		"sentence_embedding": {Name: "sentence_embedding", Rank: 2, Dim: 384},
		"last_hidden_state":  {Name: "last_hidden_state", Rank: 3, Dim: 384},
	}
	name, _ := selectOutputTensor(outputs)
	if name != "sentence_embedding" {
		t.Fatalf("expected sentence_embedding (rank 2), got %s", name)
	}
}

func TestSelectOutputTensorEmpty(t *testing.T) {
	name, _ := selectOutputTensor(map[string]onnx.OutputInfo{})
	if name != "last_hidden_state" {
		t.Fatalf("expected fallback last_hidden_state, got %s", name)
	}
}

func TestPoolingForRank2(t *testing.T) {
	if poolingForRank(2) != "none" {
		t.Fatalf("expected none, got %s", poolingForRank(2))
	}
}

func TestPoolingForRank3(t *testing.T) {
	if poolingForRank(3) != "mean" {
		t.Fatalf("expected mean, got %s", poolingForRank(3))
	}
}

func TestPoolingForRankOther(t *testing.T) {
	if poolingForRank(4) != "mean" {
		t.Fatalf("expected mean for rank 4, got %s", poolingForRank(4))
	}
}
