package embverify

import (
	"errors"
	"fmt"

	"github.com/elcuervo/emb/internal/resp"
)

// Pair is one model/text request.
type Pair struct {
	Model string
	Text  string
}

// Embedder issues EMB commands over a shared RESP client and returns decoded
// float32 vectors.
type Embedder struct {
	c *resp.Client
}

// NewEmbedder wraps a connected client.
func NewEmbedder(c *resp.Client) *Embedder { return &Embedder{c: c} }

// RawEmbed returns one text's raw embedding payload bytes (no float decode), so
// callers that need byte-level comparison do not round-trip through float32.
func (e *Embedder) RawEmbed(model, text string) ([]byte, error) {
	if err := e.c.WriteArgv("EMB", model, text); err != nil {
		return nil, err
	}
	if err := e.c.Flush(); err != nil {
		return nil, err
	}
	rep, err := e.c.ReadReply()
	if err != nil {
		return nil, err
	}
	if err := rep.Err(); err != nil {
		return nil, err
	}
	return RawVectorFromReply(rep)
}

// Embed returns one text's embedding as float32. It surfaces server errors and
// rejects a reply that is not exactly one vector.
func (e *Embedder) Embed(model, text string) ([]float32, error) {
	raw, err := e.RawEmbed(model, text)
	if err != nil {
		return nil, err
	}
	return DecodeFloat32(raw)
}

// EmbedAll embeds texts in order, stopping at the first failure and naming the
// text index.
func (e *Embedder) EmbedAll(model string, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := e.Embed(model, t)
		if err != nil {
			return nil, fmt.Errorf("text %d: %w", i, err)
		}
		out[i] = v
	}
	return out, nil
}

// RawMultiEmbed runs one EMB.MULTI for the pairs and returns each pair's raw
// payload, with nil for a failed pair (MGET semantics). A transport or protocol
// error fails the whole call.
func (e *Embedder) RawMultiEmbed(pairs []Pair) ([][]byte, error) {
	args := make([]string, 0, 1+2*len(pairs))
	args = append(args, "EMB.MULTI")
	for _, p := range pairs {
		args = append(args, p.Model, p.Text)
	}
	if err := e.c.WriteArgv(args...); err != nil {
		return nil, err
	}
	if err := e.c.Flush(); err != nil {
		return nil, err
	}
	rep, err := e.c.ReadReply()
	if err != nil {
		return nil, err
	}
	if err := rep.Err(); err != nil {
		return nil, err
	}
	if rep.Type != '*' || rep.Nil {
		return nil, fmt.Errorf("embverify: EMB.MULTI returned %q, want an array", rep.Type)
	}
	out := make([][]byte, len(rep.Elems))
	for i, el := range rep.Elems {
		if el.Nil {
			continue
		}
		raw, err := RawVectorFromReply(el)
		if err != nil {
			continue // a per-pair failure is a null slot, not a call failure
		}
		out[i] = raw
	}
	return out, nil
}

// RawVectorFromReply returns the raw payload of a bulk reply, or of a
// single-element array holding one (how a multi-text-shaped reply can arrive).
func RawVectorFromReply(rep resp.Reply) ([]byte, error) {
	switch rep.Type {
	case '$':
		if rep.Nil {
			return nil, errors.New("embverify: server returned a nil embedding")
		}
		return []byte(rep.Str), nil
	case '*':
		if rep.Nil {
			return nil, errors.New("embverify: server returned a nil array")
		}
		if len(rep.Elems) != 1 {
			return nil, fmt.Errorf("embverify: expected one embedding, got %d", len(rep.Elems))
		}
		return RawVectorFromReply(rep.Elems[0])
	default:
		return nil, fmt.Errorf("embverify: unexpected reply type %q", rep.Type)
	}
}
