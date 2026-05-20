package locales

import "embed"

// FS holds shipped locale JSON files (en.json, de.json, …).
//
//go:embed *.json
var FS embed.FS
