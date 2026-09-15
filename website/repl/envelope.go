package main

import (
	"encoding/base64"
	"unicode/utf8"

	"github.com/elcuervo/emb/internal/resp"
)

// Envelope is one reply, discriminated by Kind so a client never re-parses a
// wire format. A map's Elems hold alternating key/value envelopes in wire
// order (the shape RESP3 delivers); a binary embedding is Kind "bulk" with B64
// plus a Vector descriptor, so the client can present it without decoding a
// model. Error replies carry Code, which distinguishes a refusal from a
// transport, readiness, capacity, or timeout failure.
type Envelope struct {
	Kind   string     `json:"kind"`
	Text   string     `json:"text,omitempty"`
	B64    string     `json:"b64,omitempty"`
	Float  *float64   `json:"float,omitempty"`
	Int    *int64     `json:"int,omitempty"`
	Elems  []Envelope `json:"elems,omitempty"`
	Vector *Vector    `json:"vector,omitempty"`
	Code   string     `json:"code,omitempty"`
}

// Vector describes a bulk that carries an embedding: its element dtype and
// element count.
type Vector struct {
	Dtype string `json:"dtype"`
	Count int    `json:"count"`
}

// Reply kinds, matching the discriminator values clients switch on.
const (
	kindBulk   = "bulk"
	kindStatus = "status"
	kindInt    = "int"
	kindDouble = "double"
	kindNil    = "nil"
	kindError  = "error"
	kindArray  = "array"
	kindMap    = "map"
)

// error codes for replies the bridge itself produces.
const (
	codeRefused     = "refused"     // outside the fixed showcase surface
	codeStarting    = "starting"    // upstream not answering yet
	codeUnavailable = "unavailable" // upstream was ready and is gone
	codeCapacity    = "capacity"    // a spend bound refused the request
	codeTimeout     = "timeout"     // the upstream outlived its deadline
)

// toEnvelope converts an upstream reply. blob reports that the request asked
// for a binary embedding (an EMB/EMB.MULTI BLOB reply): its bulk payloads are
// bytes, never text, so they are always carried base64 with a vector
// descriptor rather than round-tripped through a text encoding that could
// replace an invalid sequence.
func toEnvelope(r resp.Reply, blob bool) Envelope {
	switch r.Type {
	case '+':
		return Envelope{Kind: kindStatus, Text: r.Str}
	case '-':
		return Envelope{Kind: kindError, Text: r.Str}
	case ':':
		n := r.Int
		return Envelope{Kind: kindInt, Int: &n}
	case '$':
		// A nil bulk is spelled two ways (RESP2 $-1, RESP3 _); both are a null.
		if r.Nil {
			return Envelope{Kind: kindNil}
		}
		if blob || !utf8.ValidString(r.Str) {
			env := Envelope{Kind: kindBulk, B64: base64.StdEncoding.EncodeToString([]byte(r.Str))}
			if n := len(r.Str); n%4 == 0 {
				env.Vector = &Vector{Dtype: "float32", Count: n / 4}
			}
			return env
		}
		return Envelope{Kind: kindBulk, Text: r.Str}
	case ',':
		f := r.Float
		return Envelope{Kind: kindDouble, Float: &f}
	case '_':
		return Envelope{Kind: kindNil}
	case '*':
		if r.Nil {
			return Envelope{Kind: kindNil}
		}
		elems := make([]Envelope, len(r.Elems))
		for i, el := range r.Elems {
			elems[i] = toEnvelope(el, blob)
		}
		return Envelope{Kind: kindArray, Elems: elems}
	case '%':
		elems := make([]Envelope, len(r.Elems))
		for i, el := range r.Elems {
			elems[i] = toEnvelope(el, blob)
		}
		return Envelope{Kind: kindMap, Elems: elems}
	default:
		return Envelope{Kind: kindError, Text: "unknown reply kind", Code: codeUnavailable}
	}
}

// errorEnvelope builds a legible error reply in the same discriminated shape
// as a command result, so a client renders a refusal exactly where it renders
// a value.
func errorEnvelope(code, text string) Envelope {
	return Envelope{Kind: kindError, Code: code, Text: text}
}
