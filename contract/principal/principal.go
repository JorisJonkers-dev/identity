// Package principal is identity's Principal contract: who is calling an application, the same on
// every channel. An adapter pins a version of this module and reads its files.
package principal

import "embed"

// Files holds principal.schema.json, mapping.json and the conformance vectors under vectors/.
//
//go:embed principal.schema.json mapping.json vectors
var Files embed.FS
