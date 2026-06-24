// Package migrations embeds the goose SQL migration files so they can be
// applied programmatically at service startup without shipping the .sql files
// or invoking the goose CLI.
package migrations

import "embed"

// FS holds every goose migration in this directory at the filesystem root,
// which is the layout goose's provider expects.
//
//go:embed *.sql
var FS embed.FS
