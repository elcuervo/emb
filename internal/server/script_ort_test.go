package server

import (
	"os"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

// ortOK reports whether the ONNX Runtime shared library was initialized for
// this test process. Scripted-model tests exercise real ORT sessions (via the
// minilm/gliner fixtures); they skip when the library is unavailable rather
// than failing in environments without the nix dev shell.
var ortOK bool

func TestMain(m *testing.M) {
	ortOK = onnx.InitEnvironment("") == nil
	code := m.Run()
	if ortOK {
		_ = onnx.DestroyEnvironment()
	}
	os.Exit(code)
}
