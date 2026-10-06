# Design

## Context

`fixedCol` right-aligns a whole segment, which is right for a bare number like
`0.0 r/s` (the unit stays put) but wrong for a segment that carries a label: the
label is part of the padded string, so `avg 0µs` and `p50 16.9ms` place their
labels at different columns.

## Goals / Non-Goals

**Goals:**

- `p50`, `p95`, `avg` and `err` labels on fixed columns across rows and states.
- Values still cannot move the columns after them.

**Non-Goals:**

- Right-aligning the labelled values; the value follows its label.

## Decisions

**A `fixedLabeledCol` helper.** `label + " " + value`, padded to the slot width.
The label takes the slot's left edge; the value follows and the slot's tail is
blank. Both the label column and the value's start column are then functions of
the slot alone.

**Bare numbers keep `fixedCol`.** The rates have no label — `0.0 r/s` is a value
and its unit — so right-aligning them keeps the unit on a fixed column, which is
what a numeric column should do.

## Risks / Trade-offs

- The duration units no longer share a right edge (`3µs` and `16.9ms` start at
  the same column, not end at the same one). The labels sharing a column matters
  more than the units, and the value's own column is what the eye tracks.
