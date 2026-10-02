package script

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ConfigDigest returns a stable digest of a script's declared config, folded
// into its reply-cache identity so an edited config cannot serve a reply
// computed under the old one. It is empty when there is no config, which keeps
// no-config scripts' cache keys byte-identical to the pre-config format.
//
// encoding/json sorts map keys, so the digest is deterministic across runs and
// processes.
func ConfigDigest(cfg map[string]any) string {
	if len(cfg) == 0 {
		return ""
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
