// Package skills embeds the agent skills shipped with ohmylaya so the
// installer can write them without any external file.
package skills

import "embed"

// FS holds skills/<name>/SKILL.md and their references.
//
//go:embed ohmylaya/SKILL.md ohmylaya/references/*.md
var FS embed.FS
