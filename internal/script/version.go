package script

// emb has ONE version: the repository VERSION file, injected into the binary
// and set here by the server. There is no separate script API version. Scripts
// reading emb.API_VERSION therefore learn the server they are talking to, and
// the same value is folded into reply-cache identity so an upgrade that changes
// host semantics can never serve a reply computed under the old surface.
//
// It is a package variable rather than a constant because the value comes from
// the build (main.version via Server.SetVersion), not from source. SetVersion
// is called once during startup, before any connection is served, so the read
// on the evaluation path is never racing a write.
var APIVersion = DefaultVersion

// DefaultVersion is reported when no build version was injected: `go test`,
// `go run`, and any build without -ldflags -X main.version. INFO reports the
// same value as emb_version, so the two can never disagree.
const DefaultVersion = "dev"

// SetVersion sets the version reported as emb.API_VERSION and folded into
// reply-cache keys. An empty value (a build with no ldflag) keeps
// DefaultVersion. It must be called before the server serves requests.
func SetVersion(v string) {
	if v == "" {
		v = DefaultVersion
	}
	APIVersion = v
}
