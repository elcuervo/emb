package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
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

// errTransport marks a failure of the connection itself rather than a reply the
// server sent. The connection is gone and a redial is the only recovery, so it
// is the one failure worth retrying: emb closes a connection idle for its
// `idle_timeout` (15m by default), and the zone's client is idle whenever the
// zone is quiet.
var errTransport = errors.New("upstream: connection lost")

// Upstream is the emb server that does the embedding. The zone holds no model
// of its own: it asks this one for the vectors and ranks what comes back.
type Upstream struct {
	client *resp.Client
	addr   string
	model  string
	// sha identifies the composition preset on the server. It is learned from
	// the server rather than computed here, because the server's identity for a
	// script is its own.
	sha        string
	scriptPath string
	// mu holds the one connection for the length of a call. The zone serves
	// queries concurrently and a RESP exchange is a write, a flush, and a read
	// on one socket, so two callers sharing it would interleave replies.
	mu sync.Mutex
}

// Dial connects to the emb server and loads the composition preset.
func Dial(cfg Config) (*Upstream, error) {
	u := &Upstream{
		client:     resp.NewClient(cfg.Upstream, cfg.Password, false),
		addr:       cfg.Upstream,
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

// reconnect dials again after the connection was lost and reloads the preset,
// because the server that answers the new connection may be a restarted one
// that has forgotten it. The caller holds the lock.
func (u *Upstream) reconnect() error {
	// Dial closes whatever is left of the previous connection.
	if err := u.client.Dial(); err != nil {
		return fmt.Errorf("upstream: redial %s: %w", u.addr, err)
	}
	if u.scriptPath == "" {
		return nil
	}
	sha, err := u.loadScript()
	if err != nil {
		return err
	}
	u.sha = sha
	return nil
}

// exchange writes argv, flushes, and reads one reply. A failure of the
// connection itself is marked, so a caller can tell it from a reply the server
// sent and decide whether a redial is worth it.
func (u *Upstream) exchange(argv []string, deadline time.Duration) (resp.Reply, error) {
	if err := u.client.SetDeadline(time.Now().Add(deadline)); err != nil {
		return resp.Reply{}, fmt.Errorf("upstream: %w", err)
	}
	if err := u.client.WriteArgv(argv...); err != nil {
		return resp.Reply{}, fmt.Errorf("%w: %v", errTransport, err)
	}
	if err := u.client.Flush(); err != nil {
		return resp.Reply{}, fmt.Errorf("%w: %v", errTransport, err)
	}
	reply, err := u.client.ReadReply()
	if err != nil {
		return resp.Reply{}, fmt.Errorf("%w: %v", errTransport, err)
	}
	return reply, nil
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
	u.mu.Lock()
	defer u.mu.Unlock()
	vectors, err := u.embed(texts)
	if err == nil || !errors.Is(err, errTransport) {
		return vectors, err
	}
	// The connection is gone: redial once and redo the batch. Embedding is
	// idempotent, so a retry costs the round trips and not a wrong answer.
	if rerr := u.reconnect(); rerr != nil {
		return nil, err
	}
	return u.embed(texts)
}

func (u *Upstream) embed(texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += bootBatch {
		end := min(start+bootBatch, len(texts))
		chunk := texts[start:end]
		if err := u.client.SetDeadline(time.Now().Add(bootDeadline)); err != nil {
			return nil, fmt.Errorf("%w: %v", errTransport, err)
		}
		for _, text := range chunk {
			if err := u.client.WriteArgv("EMB", u.model, text); err != nil {
				return nil, fmt.Errorf("%w: %v", errTransport, err)
			}
		}
		if err := u.client.Flush(); err != nil {
			return nil, fmt.Errorf("%w: %v", errTransport, err)
		}
		for i, text := range chunk {
			reply, err := u.client.ReadReply()
			if err != nil {
				return nil, fmt.Errorf("%w: %v", errTransport, err)
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
	u.mu.Lock()
	defer u.mu.Unlock()
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
		reply, err := u.exchange(argv, resp.DefaultTimeout)
		if err != nil {
			// The connection is gone — the server closed an idle one, or it
			// restarted. Redial once and ask again.
			if errors.Is(err, errTransport) && attempt == 0 {
				if u.reconnect() == nil {
					continue
				}
			}
			return nil, err
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
