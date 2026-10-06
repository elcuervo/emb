package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/elcuervo/emb/internal/emoji"
	"github.com/elcuervo/emb/internal/resp"
)

// bootBatch is how many texts one pipelined round trip carries while the
// index is built. The server's batcher coalesces them into few ONNX runs, so a
// larger batch buys fewer round trips rather than more inference.
const bootBatch = 64

// bootDeadline bounds one pipelined index-build round trip. A cold model on a
// small machine is slower than a query, so this is generous where the query
// path is not.
const bootDeadline = 120 * time.Second

// Upstream is the emb server that does the embedding. The zone holds no model
// of its own: it asks this one for the vectors and ranks what comes back.
type Upstream struct {
	client *resp.Client
	model  string
	// sha identifies the composition preset on the server. It is learned from
	// the server rather than computed here, because the server's identity for a
	// script is its own.
	sha        string
	scriptPath string
}

// Dial connects to the emb server and loads the composition preset.
func Dial(cfg Config) (*Upstream, error) {
	u := &Upstream{
		client:     resp.NewClient(cfg.Upstream, cfg.Password, false),
		model:      cfg.Model,
		scriptPath: cfg.Script,
	}
	if err := u.client.Dial(); err != nil {
		return nil, fmt.Errorf("upstream %s: %w", cfg.Upstream, err)
	}
	if cfg.Script != "" {
		sha, err := u.loadScript()
		if err != nil {
			return nil, err
		}
		u.sha = sha
	}
	return u, nil
}

// Close releases the upstream connection.
func (u *Upstream) Close() error { return u.client.Close() }

// loadScript registers the composition preset and returns the digest the
// server identifies it by.
func (u *Upstream) loadScript() (string, error) {
	// #nosec G703 -- the script path is operator configuration, never request data.
	source, err := os.ReadFile(u.scriptPath)
	if err != nil {
		return "", fmt.Errorf("script %s: %w", u.scriptPath, err)
	}
	if err := u.client.WriteArgv("EMB.SCRIPT", "LOAD", u.model, string(source)); err != nil {
		return "", fmt.Errorf("upstream: %w", err)
	}
	if err := u.client.Flush(); err != nil {
		return "", fmt.Errorf("upstream: %w", err)
	}
	reply, err := u.client.ReadReply()
	if err != nil {
		return "", fmt.Errorf("upstream: %w", err)
	}
	if err := reply.Err(); err != nil {
		return "", fmt.Errorf("upstream: load %s: %w", u.scriptPath, err)
	}
	sha := strings.TrimSpace(reply.Str)
	if sha == "" {
		return "", fmt.Errorf("upstream: load %s: no digest returned", u.scriptPath)
	}
	return sha, nil
}

// Embed returns one vector per text, pipelining the batch into few round trips
// so the server's own batcher does the work. It is the index build's path: it
// asks for the model's embedding of the vocabulary's descriptions.
func (u *Upstream) Embed(texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += bootBatch {
		end := min(start+bootBatch, len(texts))
		chunk := texts[start:end]
		if err := u.client.SetDeadline(time.Now().Add(bootDeadline)); err != nil {
			return nil, fmt.Errorf("upstream: %w", err)
		}
		for _, text := range chunk {
			if err := u.client.WriteArgv("EMB", u.model, text); err != nil {
				return nil, fmt.Errorf("upstream: %w", err)
			}
		}
		if err := u.client.Flush(); err != nil {
			return nil, fmt.Errorf("upstream: %w", err)
		}
		for i, text := range chunk {
			reply, err := u.client.ReadReply()
			if err != nil {
				return nil, fmt.Errorf("upstream: %w", err)
			}
			if err := reply.Err(); err != nil {
				return nil, fmt.Errorf("upstream: embed %q: %w", text, err)
			}
			vector, err := decodeVector([]byte(reply.Str))
			if err != nil {
				return nil, fmt.Errorf("upstream: embed %q: %w", text, err)
			}
			vectors[start+i] = vector
		}
	}
	return vectors, nil
}

// Compose turns a query's terms into one vector by running the preset, which
// embeds each term through the server's embedding path and folds them with the
// `+`/`-` the query wrote. A single-term query is that term's vector.
func (u *Upstream) Compose(query emoji.Query) ([]float32, error) {
	if len(query.Terms) == 0 {
		return nil, emoji.ErrNoTerms
	}
	if u.sha == "" {
		return nil, fmt.Errorf("upstream: no composition script is loaded")
	}
	argv := make([]string, 0, 4+2*len(query.Terms))
	argv = append(argv, "EMB.EVSHA", u.model, u.sha, "1", query.Terms[0].Text)
	for _, term := range query.Terms[1:] {
		operator := "+"
		if term.Subtract {
			operator = "-"
		}
		argv = append(argv, operator, term.Text)
	}

	for attempt := range 2 {
		if err := u.client.SetDeadline(time.Now().Add(resp.DefaultTimeout)); err != nil {
			return nil, fmt.Errorf("upstream: %w", err)
		}
		if err := u.client.WriteArgv(argv...); err != nil {
			return nil, fmt.Errorf("upstream: %w", err)
		}
		if err := u.client.Flush(); err != nil {
			return nil, fmt.Errorf("upstream: %w", err)
		}
		reply, err := u.client.ReadReply()
		if err != nil {
			return nil, fmt.Errorf("upstream: %w", err)
		}
		if reply.Err() != nil {
			// A restarted server has forgotten its scripts; load and retry
			// once, so a restart does not take the zone down with it.
			if attempt == 0 && strings.Contains(reply.Str, "NOSCRIPT") && u.scriptPath != "" {
				sha, err := u.loadScript()
				if err != nil {
					return nil, err
				}
				u.sha = sha
				continue
			}
			return nil, fmt.Errorf("upstream: %w", reply.Err())
		}
		return decodeVector([]byte(reply.Str))
	}
	return nil, fmt.Errorf("upstream: the composition preset could not be loaded")
}

// decodeVector reads the little-endian float32 bulk an emb reply carries.
func decodeVector(raw []byte) ([]float32, error) {
	if len(raw) == 0 || len(raw)%4 != 0 {
		return nil, fmt.Errorf("upstream: expected a float32 vector, got %d bytes", len(raw))
	}
	vector := make([]float32, len(raw)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return vector, nil
}
