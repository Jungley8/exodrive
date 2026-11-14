package plugins

import (
	"path/filepath"

	"exodrive/internal/core"
)

// NewAntigravityProvider moves the Antigravity CLI session store. The CLI reads
// ~/.gemini/antigravity-cli on every start and follows a symbolic link, so the
// moved folders need no flag and no setting.
//
// The folders sit under the app data root, next to settings and the keychain
// reference. Exodrive moves only the two folders that hold conversations, and
// leaves every sibling in place.
func NewAntigravityProvider() *linkedProvider {
	return &linkedProvider{
		tool: "antigravity",
		lock: []string{"agy"},
		rows: []linkedRow{
			{
				id:       "antigravity.conversations",
				title:    "Antigravity Conversations",
				path:     func() string { return homePath(".gemini", "antigravity-cli", "conversations") },
				category: core.CategorySessionStore,
				subdir:   filepath.Join("antigravity", "conversations"),
				warning:  "Antigravity deletes old conversations automatically.",
			},
			{
				id:       "antigravity.brain",
				title:    "Antigravity Brain",
				path:     func() string { return homePath(".gemini", "antigravity-cli", "brain") },
				category: core.CategorySessionStore,
				subdir:   filepath.Join("antigravity", "brain"),
				warning:  "Antigravity writes task logs and plans here for each conversation.",
			},
		},
	}
}
