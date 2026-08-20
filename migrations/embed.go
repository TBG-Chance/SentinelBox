package migrations

import "embed"

// Files contains the forward-only production migrations compiled into the
// SentinelBox binary.
//
//go:embed *.up.sql
var Files embed.FS
