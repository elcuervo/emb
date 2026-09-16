package main

import (
	"strings"
	"testing"
)

func TestAnsiHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "hello world", "hello world"},
		{"markup in the frame is escaped", "a<b>&\"c", "a&lt;b&gt;&amp;&#34;c"},
		{"truecolor foreground", "\x1b[38;2;255;90;31mhi\x1b[0m", `<span style="color:#ff5a1f">hi</span>`},
		{"truecolor background keeps the heatmap cell", "\x1b[48;2;24;24;37m \x1b[0m", `<span style="background-color:#181825"> </span>`},
		{"bold", "\x1b[1mx\x1b[0m", `<span style="font-weight:700">x</span>`},
		{"dim and italic combine", "\x1b[2;3mx\x1b[0m", `<span style="font-style:italic;opacity:.6">x</span>`},
		{"indexed colour", "\x1b[38;5;196mX\x1b[0m", `<span style="color:#ff0000">X</span>`},
		{"basic colour", "\x1b[31mA\x1b[0mB", `<span style="color:#cd0000">A</span>B`},
		{"reset returns to the page's own ink", "\x1b[38;2;1;2;3mA\x1b[0mB", `<span style="color:#010203">A</span>B`},
		{"default foreground clears the colour", "\x1b[38;2;1;2;3mA\x1b[39mB", `<span style="color:#010203">A</span>B`},
		{"an escape the frame does not carry is dropped", "\x1b[?25lhello", "hello"},
		{"newlines survive for the pre", "a\nb", "a\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ansiHTML(tc.in); got != tc.want {
				t.Fatalf("ansiHTML(%q):\n got %s\nwant %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestAnsiHTMLRealFrameHasNoResidualEscapes renders a frame built from the
// sequences the dashboard actually emits and asserts nothing terminal-control
// survives into the page.
func TestAnsiHTMLRealFrameHasNoResidualEscapes(t *testing.T) {
	frame := strings.Join([]string{
		"\x1b[1memb-top v0.4.0\x1b[0m \x1b[38;2;137;180;250mlocalhost:6379\x1b[0m \x1b[38;2;166;227;161m42.0 r/s\x1b[0m",
		"\x1b[38;2;249;226;175mreq/s · models × recent polls\x1b[0m",
		"\x1b[38;2;137;180;250mminilm      \x1b[0m\x1b[48;2;243;139;168m█\x1b[0m\x1b[48;2;166;227;161m█\x1b[0m\x1b[48;2;24;24;37m \x1b[0m",
		"\x1b[38;2;137;180;250mminilm      \x1b[0m \x1b[38;2;166;227;161m12.3 r/s\x1b[0m \x1b[38;2;108;108;110merr 0\x1b[0m",
		"cache 97.0% · conns 3 · active 1",
	}, "\n")

	out := ansiHTML(frame)
	if strings.ContainsRune(out, 0x1b) {
		t.Fatalf("residual escape sequence in markup: %q", out)
	}
	if !strings.Contains(out, "background-color:#f38ba8") {
		t.Fatalf("heatmap cell colour was lost:\n%s", out)
	}
	if !strings.Contains(out, "minilm") || !strings.Contains(out, "cache 97.0%") {
		t.Fatalf("frame text was lost:\n%s", out)
	}
}
