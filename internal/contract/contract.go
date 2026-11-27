// Package contract is emb's command surface: the single declaration every
// other representation (commands.json, client validation) derives from. It is
// stdlib-only so generators build with CGO_ENABLED=0 and without ONNX.
package contract

import (
	"encoding/json"
	"sort"
)

type Flag string

const (
	FlagInference        Flag = "inference"
	FlagIntrospection    Flag = "introspection"
	FlagWritesState      Flag = "writes-state"
	FlagChangesConfig    Flag = "changes-config"
	FlagExposesTraffic   Flag = "exposes-traffic"
	FlagTerminatesServer Flag = "terminates-server"
)

// Arity counts arguments after the command name.
type Arity struct {
	Min  int    `json:"min"`
	Tail string `json:"tail"` // "none" | "pairs" | "even-pairs" | "variadic"
}

type Doc struct {
	Name        string          `json:"name"`
	Summary     string          `json:"summary"`
	Grammar     string          `json:"grammar"`
	Keywords    []string        `json:"keywords"`
	Arity       Arity           `json:"arity"`
	Flags       []Flag          `json:"flags"`
	Reply       string          `json:"reply"`
	ReplySchema json.RawMessage `json:"reply_schema,omitempty"`
	WireNotes   string          `json:"wire_notes,omitempty"`
}

const imageSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "oneOf": [
    {
      "type": "null",
      "description": "one image that failed or was truncated"
    },
    {
      "type": "object",
      "description": "one image: output name to that output's value"
    },
    {
      "type": "array",
      "description": "several images, one keyed map per image in request order",
      "items": {
        "oneOf": [
          {"type": "object"},
          {"type": "null"}
        ]
      }
    }
  ]
}`

// imageMultiSchema is array-only: writeIMGMULTIResult always writes one slot per
// pair, so a collapsed single map is never a valid reply.
const imageMultiSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "array",
  "description": "one keyed map per model/image pair in request order",
  "items": {
    "oneOf": [
      {"type": "object"},
      {"type": "null"}
    ]
  }
}`

const imageWireNotes = "RESP2 writes each map as a flat alternating name/value array; RESP3 writes a map. VALUES replaces each bulk string with a dtype/shape/values envelope."

// Docs is every served command, sorted by Name.
var Docs = []Doc{
	{
		Name:     "AUTH",
		Summary:  "Authenticate the connection when a password is configured.",
		Grammar:  "AUTH <password>",
		Keywords: []string{},
		Arity:    Arity{Min: 1, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "OK on success; an error otherwise.",
	},
	{
		Name:     "CLIENT",
		Summary:  "Answer the CLIENT subcommands RESP3 client handshakes send.",
		Grammar:  "CLIENT SETINFO <name> <value>",
		Keywords: []string{"SETINFO"},
		Arity:    Arity{Min: 1, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "OK for CLIENT SETINFO; an error for unknown subcommands.",
	},
	{
		Name:     "CONFIG",
		Summary:  "Read or live-tune runtime configuration parameters.",
		Grammar:  "CONFIG GET [pattern] | CONFIG SET <param> <value>",
		Keywords: []string{"GET", "SET"},
		Arity:    Arity{Min: 1, Tail: "none"},
		Flags:    []Flag{FlagChangesConfig},
		Reply:    "GET replies with flat name/value pairs; SET replies OK or an error.",
	},
	{
		Name:     "EMB",
		Summary:  "Generate embeddings for one or more texts of one model.",
		Grammar:  "EMB <model> [BLOB|VALUES] <text> [text...]",
		Keywords: []string{"BLOB", "VALUES"},
		Arity:    Arity{Min: 2, Tail: "variadic"},
		Flags:    []Flag{FlagInference},
		Reply:    "One text: a bulk string of float32 bytes (BLOB) or a VALUES envelope. Several texts under BLOB: an array of the same in request order. Several texts under VALUES: one envelope with shape [count, dim].",
	},
	{
		Name:     "EMB.CACHE.FLUSH",
		Summary:  "Invalidate all cached embeddings, or one model's.",
		Grammar:  "EMB.CACHE.FLUSH [model]",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagWritesState},
		Reply:    "Integer count of removed entries.",
	},
	{
		Name:     "EMB.EVAL",
		Summary:  "Evaluate a Lua script once against a model.",
		Grammar:  "EMB.EVAL <model> <script> <numtexts> <text...> <arg...>",
		Keywords: []string{},
		Arity:    Arity{Min: 3, Tail: "variadic"},
		Flags:    []Flag{FlagInference},
		Reply:    "The script's converted value, or an array of values for several texts.",
	},
	{
		Name:     "EMB.EVSHA",
		Summary:  "Evaluate a cached script by SHA1.",
		Grammar:  "EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>",
		Keywords: []string{},
		Arity:    Arity{Min: 3, Tail: "variadic"},
		Flags:    []Flag{FlagInference},
		Reply:    "The script's converted value, or an array of values for several texts.",
	},
	{
		Name:     "EMB.HELP",
		Summary:  "Show the command reference, including the script surface.",
		Grammar:  "EMB.HELP",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "A bulk string of help lines.",
	},
	{
		Name:        "EMB.IMG",
		Summary:     "Embed one or more raw encoded images of one model.",
		Grammar:     "EMB.IMG <model> [BLOB|VALUES] [OUTPUTS <name>[,<name>...]] <image-bytes> [<image-bytes>...]",
		Keywords:    []string{"BLOB", "VALUES", "OUTPUTS"},
		Arity:       Arity{Min: 2, Tail: "variadic"},
		Flags:       []Flag{FlagInference},
		Reply:       "One image: a keyed map of output name to value; several: an array of such maps in request order, with null for a failed or truncated slot.",
		ReplySchema: json.RawMessage(imageSchema),
		WireNotes:   imageWireNotes,
	},
	{
		Name:        "EMB.IMGMULTI",
		Summary:     "Embed images across different models in one call.",
		Grammar:     "EMB.IMGMULTI [BLOB|VALUES] [OUTPUTS <name>[,<name>...]] <model> <image-bytes> [<model> <image-bytes>...]",
		Keywords:    []string{"BLOB", "VALUES", "OUTPUTS"},
		Arity:       Arity{Min: 2, Tail: "even-pairs"},
		Flags:       []Flag{FlagInference},
		Reply:       "An array with one keyed map per pair in request order; a failed pair is null; under VALUES each pair carries its model beside its outputs.",
		ReplySchema: json.RawMessage(imageMultiSchema),
		WireNotes:   imageWireNotes,
	},
	{
		Name:     "EMB.INFO",
		Summary:  "Show one model's configuration and statistics.",
		Grammar:  "EMB.INFO <model>",
		Keywords: []string{},
		Arity:    Arity{Min: 1, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "A map of model fields: dim, workers, latency, CPU plan, embed/image/script phases, image outputs, and cache stats.",
	},
	{
		Name:     "EMB.MODELS",
		Summary:  "List loaded models with dimensions and status.",
		Grammar:  "EMB.MODELS",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "RESP2: an array of [name, dim, status]; RESP3: a map keyed by model name with dim and status.",
	},
	{
		Name:     "EMB.MULTI",
		Summary:  "Embed texts across different models in one round trip.",
		Grammar:  "EMB.MULTI [BLOB|VALUES] <model> <text> [<model> <text>...]",
		Keywords: []string{"BLOB", "VALUES"},
		Arity:    Arity{Min: 2, Tail: "even-pairs"},
		Flags:    []Flag{FlagInference},
		Reply:    "An array with one vector (or VALUES envelope) per pair in request order; a failed pair is null.",
	},
	{
		Name:     "EMB.READY",
		Summary:  "Report server readiness.",
		Grammar:  "EMB.READY",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "+OK when ready; an error naming loading, draining, or no models.",
	},
	{
		Name:     "EMB.SAVE",
		Summary:  "Accept an asynchronous cache snapshot.",
		Grammar:  "EMB.SAVE",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagWritesState},
		Reply:    "OK once the snapshot is accepted; an error when persistence is disabled or disallowed.",
	},
	{
		Name:     "EMB.SCRIPT",
		Summary:  "Compile, inspect, and flush cached Lua scripts.",
		Grammar:  "EMB.SCRIPT LOAD <model> <script> | EMB.SCRIPT EXISTS <model> <sha...> | EMB.SCRIPT FLUSH [<model>]",
		Keywords: []string{"LOAD", "EXISTS", "FLUSH"},
		Arity:    Arity{Min: 1, Tail: "none"},
		Flags:    []Flag{FlagWritesState},
		Reply:    "LOAD replies a bulk SHA1; EXISTS replies an array of 1/0 per SHA; FLUSH replies OK.",
	},
	{
		Name:     "EMB.STATS",
		Summary:  "Show server-wide statistics.",
		Grammar:  "EMB.STATS",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "A map of uptime, requests, per-model breakdown, CPU, cache, connections, memory, and goroutines.",
	},
	{
		Name:     "HELLO",
		Summary:  "Negotiate the RESP protocol version for the connection.",
		Grammar:  "HELLO [2|3]",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "A map (flat array under RESP2) with server, version, proto, mode, and role.",
	},
	{
		Name:     "INFO",
		Summary:  "Show Redis-style server information sections.",
		Grammar:  "INFO [section...]",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "variadic"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "A bulk string of CRLF-separated sections.",
	},
	{
		Name:     "MONITOR",
		Summary:  "Show recent completed request events.",
		Grammar:  "MONITOR [seq] [limit]",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection, FlagExposesTraffic},
		Reply:    "An array of events, each a 12-field flat array under RESP2 or a map under RESP3; no text payloads.",
	},
	{
		Name:     "PING",
		Summary:  "Redis compatibility ping.",
		Grammar:  "PING",
		Keywords: []string{},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagIntrospection},
		Reply:    "Simple string PONG.",
	},
	{
		Name:     "SHUTDOWN",
		Summary:  "Stop the server with Redis's shutdown semantics.",
		Grammar:  "SHUTDOWN [NOSAVE|SAVE] [NOW] [FORCE] [ABORT]",
		Keywords: []string{"NOSAVE", "SAVE", "NOW", "FORCE", "ABORT"},
		Arity:    Arity{Min: 0, Tail: "none"},
		Flags:    []Flag{FlagTerminatesServer},
		Reply:    "No reply: the caller blocks and the connection closes. SHUTDOWN ABORT replies OK or an error.",
	},
}

func init() {
	sort.Slice(Docs, func(i, j int) bool { return Docs[i].Name < Docs[j].Name })
}

func ByName(name string) (Doc, bool) {
	for _, d := range Docs {
		if d.Name == name {
			return d, true
		}
	}
	return Doc{}, false
}

func Render() ([]byte, error) {
	out, err := json.MarshalIndent(struct {
		Contract int   `json:"contract"`
		Commands []Doc `json:"commands"`
	}{1, Docs}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
