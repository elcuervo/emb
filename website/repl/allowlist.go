package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// surface is the permitted command set, quoted in every refusal so a refusal
// points at what is available rather than being silent.
const surface = "PING, HELLO 2|3, EMB, EMB.MULTI, EMB.MODELS, EMB.INFO, EMB.STATS, EMB.READY, EMB.HELP, EMB.EVSHA (preloaded presets only), EMB.IMG (one image, the sandbox's dinov3 or depth model, BLOB only), INFO"

// presets maps a model to the digests the server preloaded for it, each to the
// preset's name. A preset is callable only when its model and digest both
// appear here, so the digest the site shows and the bytes the server loaded
// cannot disagree.
type presets map[string]map[string]string

// refusedFamilies explains why each out-of-surface command family is refused.
// The reason names the risk, not just the rule.
var refusedFamilies = map[string]string{
	"config":          "the sandbox does not expose server configuration",
	"auth":            "the sandbox holds no credentials and accepts none",
	"monitor":         "request events would describe other visitors' traffic",
	"shutdown":        "the sandbox is not visitor-stoppable",
	"emb.save":        "state writes are not visitor-triggerable",
	"emb.cache.flush": "shared state is not visitor-mutable",
	"emb.script":      "script loading and flushing are not exposed",
	"emb.eval":        "raw Lua is not accepted; call a preloaded preset by digest",
	"emb.imgmulti":    "one image per call is the whole image surface",
}

// imageModels is the pair of image branches the site exposes, each with the
// outputs its sandbox config declares: the dense-features entry (a pooled
// vector and a patch grid) and the depth entry (one 2-D map). The plate names
// them; a model the sandbox serves for text is not an image surface, and a
// third name is a config-and-allowlist change, not a widening at runtime.
var imageModels = map[string]map[string]bool{
	"dinov3": {"embedding": true, "patches": true},
	"depth":  {"depth": true},
}

// imageModelNames lists the permitted models for a refusal, sorted so the
// message is stable.
func imageModelNames() string {
	return "dinov3, depth"
}

// imageOutputList lists a model's outputs for a refusal, sorted.
func imageOutputList(outputs map[string]bool) string {
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// validateArgv checks the whole argument vector against the fixed showcase
// surface. It refuses an unknown command, a refused family, a permitted
// command whose shape was widened, and a preset call that names anything but a
// preloaded digest. bin names the arguments the bridge decoded from base64, so
// the image command can require its image to arrive as bytes rather than text.
func validateArgv(args []string, p presets, bin map[int]bool) error {
	if len(args) == 0 {
		return fmt.Errorf("empty command: the sandbox permits only %s", surface)
	}
	name := strings.ToLower(args[0])
	if why, ok := refusedFamilies[name]; ok {
		return refuse("%s is not permitted: %s", strings.ToUpper(args[0]), why)
	}
	switch name {
	case "ping":
		// PING [message] — Redis-compatible, no payload beyond one message.
		if len(args) > 2 {
			return arity(args[0])
		}
	case "hello":
		if len(args) > 2 {
			return arity(args[0])
		}
		if len(args) == 2 && args[1] != "2" && args[1] != "3" {
			return refuse("HELLO accepts only protocol version 2 or 3")
		}
	case "info":
		// INFO [section...] is a read-only server readout; sections are free-form.
	case "emb.models", "emb.stats", "emb.ready":
		if len(args) != 1 {
			return arity(args[0])
		}
	case "emb.help":
		if len(args) > 2 {
			return arity(args[0])
		}
	case "emb.info":
		// EMB.INFO <model> — exactly one model.
		if len(args) != 2 {
			return arity(args[0])
		}
	case "emb":
		// EMB <model> [BLOB|VALUES] <text>...
		if len(args) < 3 {
			return arity(args[0])
		}
		textStart := 2
		if isFormat(args[2]) {
			textStart = 3
		}
		if len(args) <= textStart {
			return refuse("EMB needs at least one text after the model")
		}
	case "emb.multi":
		// EMB.MULTI [BLOB|VALUES] <model> <text>...
		start := 1
		if len(args) > 1 && isFormat(args[1]) {
			start = 2
		}
		if len(args[start:]) < 2 || len(args[start:])%2 != 0 {
			return refuse("EMB.MULTI takes <model> <text> pairs")
		}
	case "emb.img":
		return validateImageCommand(args, bin)
	case "emb.evsha":
		// EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>
		if len(args) < 4 {
			return arity(args[0])
		}
		model, sha := args[1], args[2]
		if p[model][sha] == "" {
			return refuse("EMB.EVSHA accepts only the sandbox's preloaded presets, and %q is not one of them for model %q", sha, model)
		}
		n, err := strconv.Atoi(args[3])
		if err != nil || n < 1 {
			return refuse("EMB.EVSHA needs a positive text count")
		}
		if len(args) < 4+n {
			return refuse("EMB.EVSHA carries fewer texts than its count")
		}
	default:
		return refuse("unknown command %q", strings.ToUpper(args[0]))
	}
	return nil
}

// validateImageCommand admits exactly one EMB.IMG shape:
//
//	EMB.IMG <imageModel> BLOB [OUTPUTS <name>[,<name>...]] <one image>
//
// The image must be the command's binary argument, so a URL or a sentence is
// refused before the server could decode or fetch it, and everything else in the
// family (VALUES, EMB.IMGMULTI, another model, other outputs, a second image)
// stays out of the surface.
func validateImageCommand(args []string, bin map[int]bool) error {
	if len(args) < 4 {
		return refuse("EMB.IMG needs <model> BLOB [OUTPUTS <name>[,<name>]] <image>")
	}
	if !strings.EqualFold(args[2], "blob") {
		return refuse("EMB.IMG accepts only BLOB; a VALUES image reply is not available")
	}
	model := strings.ToLower(args[1])
	outputs, ok := imageModels[model]
	if !ok {
		return refuse("EMB.IMG serves only the sandbox's own image models (%s), not %q", imageModelNames(), args[1])
	}
	image := 3
	if strings.EqualFold(args[3], "outputs") {
		if len(args) < 6 {
			return refuse("EMB.IMG OUTPUTS needs a name list and one image")
		}
		for _, name := range strings.Split(args[4], ",") {
			if !outputs[name] {
				return refuse("EMB.IMG serves only %s's configured outputs (%s); %q is not one", model, imageOutputList(outputs), name)
			}
		}
		image = 5
	}
	if len(args) != image+1 {
		return refuse("EMB.IMG takes exactly one image per call")
	}
	if !bin[image] {
		return refuse("EMB.IMG takes the image as its binary argument (bin), not as text or a URL")
	}
	return nil
}

// isFormat reports whether arg is the BLOB or VALUES reply-format keyword. The
// server only recognizes either at one fixed position with a payload after it,
// so the bridge uses the same rule.
func isFormat(arg string) bool {
	return strings.EqualFold(arg, "blob") || strings.EqualFold(arg, "values")
}

func refuse(format string, args ...any) error {
	return fmt.Errorf("%s: the sandbox permits only %s", fmt.Sprintf(format, args...), surface)
}

func arity(name string) error {
	return refuse("wrong number of arguments for %q", strings.ToUpper(name))
}
