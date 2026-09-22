// Package db embeds the SQL assets that ship with the binary so that a single
// deployed artifact can migrate its own schema.
package db

import "embed"

//go:embed all:migrations
var migrationsFS embed.FS

// Migrations returns the embedded db/migrations directory.
func Migrations() embed.FS { return migrationsFS }
