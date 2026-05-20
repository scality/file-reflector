package version

// Version is overridden at build time via -ldflags -X. The "dev" default
// is what an unmodified `go build` produces.
//
//	go build -ldflags "-X github.com/scality/file-reflector/pkg/version.Version=v1.2.3" ./cmd
var Version = "dev"
