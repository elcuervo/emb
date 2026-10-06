# Design

## Context

The dashboard's per-model rows already reserve fixed columns (`rowLayout`) and
right-align each segment, so their values never move. The lines around them were
left as free-form `fmt.Sprintf` output, and their numbers grow during a load run,
so the text after each number slides.

## Goals / Non-Goals

**Goals:**

- Every dynamic readout on a fixed column, so a frame taken at any two polls has
  the same text in the same columns when only the numbers changed.
- No line grows wider than the terminal.

**Non-Goals:**

- Changing which values are shown.
- Making the connection-loss text (`reconnecting · last sample 3s ago`) fixed
  width; it is a state change, not a value growing.

## Decisions

**Pad at the format string.** `%5.1f`, `%3.0f`, `%-7s`, `%3d` on each dynamic
field. This is the smallest change and keeps the padding next to the value it
belongs to.

**Fixed widths sized to the real maximum.** `err %5.1f%%` covers 100.0%,
`p95 %-7s` covers `999.9ms` (the formatter switches to seconds at 1s), `cpu %3.0f%%`
covers the 0–100% the health state normalizes to, and the gauge value column is
7 cells, covering `100.0kMB`.

**The gauge value column shrinks with its band.** The strip band can be as
narrow as `minStrip`, so a fixed 7-cell value would overflow it. The column is
`min(7, bandWidth - label - 1)` and the caption is clamped to the band, so a
narrow terminal loses value width, never a column.

**The status label is padded too.** `CRITICAL` is one cell wider than `HEALTHY`,
so without padding the chips jump whenever the verdict changes.

## Risks / Trade-offs

- Trailing spaces inside a styled chip are invisible but count toward the chip's
  width, so the separator before the next chip reads as one cell wider. That is
  the price of a fixed column.
- Very large values (a four-digit memory in a narrow band) are clamped rather
  than wrapped; the digits that fit are the leading ones.
