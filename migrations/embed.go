package migrations

import "embed"

// FS contains system and partition migrations.
//
//go:embed system/*.sql partition/*.sql
var FS embed.FS
