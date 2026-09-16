package main

import (
	"html"
	"strconv"
	"strings"
)

// ansiHTML converts one `emb-top` frame — SGR-styled text with newlines — into
// the inner markup of the view's <pre>: HTML-escaped runs wrapped in spans
// carrying the foreground, background, and text attributes in effect.
//
// It handles exactly what a frame contains, which is colour and attributes and
// nothing else: no cursor addressing, no scrolling, no alternate screen, no
// erase. A frame is complete, so there is no terminal state to carry between
// calls, and a run that ends mid-style is simply not opened.
func ansiHTML(frame string) string {
	var b strings.Builder
	var st ansiStyle
	open := false
	closeSpan := func() {
		if open {
			b.WriteString("</span>")
			open = false
		}
	}
	openSpan := func() {
		if !open {
			b.WriteString(`<span style="`)
			b.WriteString(st.css())
			b.WriteString(`">`)
			open = true
		}
	}

	for i := 0; i < len(frame); {
		if frame[i] == 0x1b {
			if i+1 < len(frame) && frame[i+1] == '[' {
				// A CSI sequence ends at the first byte in 0x40..0x7e; the SGR
				// terminator is 'm'. Anything else (cursor, erase, mode) is
				// dropped whole rather than leaking its tail into the page.
				if end := csiEnd(frame, i+2); end >= 0 {
					if frame[end] == 'm' {
						st.apply(frame[i+2 : end])
						closeSpan()
					}
					i = end + 1
					continue
				}
			}
			// An escape this renderer does not carry: drop it rather than
			// leaking it into the page.
			i++
			continue
		}
		j := strings.IndexByte(frame[i:], 0x1b)
		if j < 0 {
			j = len(frame) - i
		}
		if text := frame[i : i+j]; text != "" {
			if st.styled() {
				openSpan()
			}
			b.WriteString(html.EscapeString(text))
		}
		i += j
	}
	closeSpan()
	return b.String()
}

// csiEnd returns the index of the byte that terminates the CSI sequence whose
// parameters start at i, or -1 if none is found.
func csiEnd(frame string, i int) int {
	for ; i < len(frame); i++ {
		if c := frame[i]; c >= 0x40 && c <= 0x7e {
			return i
		}
	}
	return -1
}

// ansiStyle is the SGR state accumulated across one frame.
type ansiStyle struct {
	fg, bg    string // CSS colours; "" is the page's own
	bold      bool
	dim       bool
	italic    bool
	underline bool
}

func (s ansiStyle) styled() bool {
	return s.fg != "" || s.bg != "" || s.bold || s.dim || s.italic || s.underline
}

func (s ansiStyle) css() string {
	var parts []string
	if s.fg != "" {
		parts = append(parts, "color:"+s.fg)
	}
	if s.bg != "" {
		parts = append(parts, "background-color:"+s.bg)
	}
	if s.bold {
		parts = append(parts, "font-weight:700")
	}
	if s.italic {
		parts = append(parts, "font-style:italic")
	}
	if s.underline {
		parts = append(parts, "text-decoration:underline")
	}
	if s.dim {
		parts = append(parts, "opacity:.6")
	}
	return strings.Join(parts, ";")
}

// apply folds one SGR parameter list into the style. Colours selected by
// `38`/`48` consume the parameters that follow them.
func (s *ansiStyle) apply(params string) {
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			*s = ansiStyle{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.dim = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.underline = true
		case n == 22:
			s.bold, s.dim = false, false
		case n == 23:
			s.italic = false
		case n == 24:
			s.underline = false
		case n == 39:
			s.fg = ""
		case n == 49:
			s.bg = ""
		case n >= 30 && n <= 37:
			s.fg = ansi16(n - 30)
		case n >= 90 && n <= 97:
			s.fg = ansi16(n - 90 + 8)
		case n >= 40 && n <= 47:
			s.bg = ansi16(n - 40)
		case n >= 100 && n <= 107:
			s.bg = ansi16(n - 100 + 8)
		case n == 38, n == 48:
			c, used := extendedColor(fields[i+1:])
			if used == 0 {
				continue
			}
			if n == 38 {
				s.fg = c
			} else {
				s.bg = c
			}
			i += used
		}
	}
}

// extendedColor reads an `38`/`48` payload (`5;n` or `2;r;g;b`) and reports
// how many parameters it consumed.
func extendedColor(fields []string) (string, int) {
	if len(fields) == 0 {
		return "", 0
	}
	switch fields[0] {
	case "5":
		if len(fields) < 2 {
			return "", 0
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 0 || n > 255 {
			return "", 0
		}
		return xterm256(n), 2
	case "2":
		if len(fields) < 4 {
			return "", 0
		}
		r, e1 := strconv.Atoi(fields[1])
		g, e2 := strconv.Atoi(fields[2])
		b, e3 := strconv.Atoi(fields[3])
		if e1 != nil || e2 != nil || e3 != nil {
			return "", 0
		}
		return rgb(r, g, b), 4
	}
	return "", 0
}

func rgb(r, g, b int) string {
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}
	const hex = "0123456789abcdef"
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []int{clamp(r), clamp(g), clamp(b)} {
		out[1+i*2] = hex[v>>4]
		out[2+i*2] = hex[v&0xf]
	}
	return string(out)
}

// ansi16 is the standard 16-colour palette, used only if a frame carries a
// basic colour; the frame mode forces truecolour, so the 24-bit form is what
// frames actually contain.
var ansi16Palette = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

func ansi16(n int) string { return ansi16Palette[n] }

// xterm256 maps an indexed colour to the xterm palette: the 16 base colours,
// the 6×6×6 cube, then the 24-step grey ramp.
func xterm256(n int) string {
	switch {
	case n < 16:
		return ansi16Palette[n]
	case n < 232:
		v := n - 16
		comp := func(c int) int {
			if c == 0 {
				return 0
			}
			return 55 + c*40
		}
		return rgb(comp(v/36), comp((v/6)%6), comp(v%6))
	default:
		c := 8 + (n-232)*10
		return rgb(c, c, c)
	}
}
