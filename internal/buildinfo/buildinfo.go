// Package buildinfo exposes version metadata injected at build time.
package buildinfo

// These values are overridden through -ldflags at release time.
var (
	// Version is the semantic version of the binary, or "dev".
	Version = "dev"
	// Commit is the short Git commit the binary was built from.
	Commit = "unknown"
	// Date is the build date in RFC 3339 format.
	Date = "unknown"
)
