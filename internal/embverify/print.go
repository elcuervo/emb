package embverify

import (
	"fmt"
	"io"
)

// Printf writes one report line for a verification CLI. A write failure cannot
// change a verification verdict, so the error is deliberately discarded here
// and nowhere else.
func Printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
