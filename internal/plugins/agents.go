package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// claudeFolders moves the history that Claude Code keeps under ~/.claude:
// one transcript per conversation, and the file snapshots that undo reads.
// Settings, credentials, plugins, and the keychain entry stay in place.
//
// Claude Code opens these paths on every start and follows a symbolic link, so
// the move needs no flag and no setting.
var claudeFolders = []linkedRow{
	{
		id:       "claude.projects",
		title:    "Claude Code Conversations",
		path:     func() string { return homePath(".claude", "projects") },
		category: core.CategorySessionStore,
		subdir:   filepath.Join("claude", "projects"),
		warning:  "Claude Code writes each conversation here. Quit Claude Code before the move.",
	},
	{
		id:       "claude.file-history",
		title:    "Claude Code File History",
		path:     func() string { return homePath(".claude", "file-history") },
		category: core.CategorySessionStore,
		subdir:   filepath.Join("claude", "file-history"),
		warning:  "Claude Code keeps a copy of each edited file here, so you can undo an edit.",
	},
}

// codexFolders moves the rollouts that Codex CLI writes under ~/.codex. The
// configuration, the credentials, and the plugins stay in place.
//
// Codex reads these paths on every start and follows a symbolic link.
var codexFolders = []linkedRow{
	{
		id:       "codex.sessions",
		title:    "Codex Sessions",
		path:     func() string { return homePath(".codex", "sessions") },
		category: core.CategorySessionStore,
		subdir:   filepath.Join("codex", "sessions"),
		warning:  "Codex writes each conversation here. Quit Codex before the move.",
	},
	{
		id:       "codex.archived-sessions",
		title:    "Codex Archived Sessions",
		path:     func() string { return homePath(".codex", "archived_sessions") },
		category: core.CategorySessionStore,
		subdir:   filepath.Join("codex", "archived-sessions"),
		warning:  "Codex moves conversations that you archive to this folder.",
	},
}

// opencodeDataRoot returns the folder where OpenCode keeps its data. The
// program follows the XDG base directory rule on macOS and on Linux.
func opencodeDataRoot() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dir != "" {
		return filepath.Join(dir, "opencode")
	}
	return homePath(".local", "share", "opencode")
}

// noPendingJournal refuses a move of a database that still has pages in its
// journal, because Exodrive moves the database file alone and those pages
// would stay behind.
func noPendingJournal(path string) error {
	journal := path + "-wal"
	info, err := os.Stat(journal)
	if err != nil || info.Size() == 0 {
		return nil
	}
	return fmt.Errorf("%s holds %d bytes that the database file does not: start OpenCode once, quit it, then move again",
		filepath.Base(journal), info.Size())
}

// opencodeRows moves the store that OpenCode fills: one database with every
// conversation, the file snapshots behind undo, and the older per-folder
// records. The binaries, the credentials, and the configuration stay in place.
//
// OpenCode opens these paths on every start and follows a symbolic link. It
// also keeps a background service that holds the database open, so the process
// lock covers the service too.
var opencodeRows = []linkedRow{
	{
		id:       "opencode.database",
		title:    "OpenCode Conversations",
		path:     func() string { return filepath.Join(opencodeDataRoot(), "opencode.db") },
		category: core.CategorySessionStore,
		subdir:   "opencode",
		file:     true,
		check:    noPendingJournal,
		warning:  "OpenCode keeps every conversation in this database. Quit OpenCode before the move.",
	},
	{
		id:       "opencode.snapshot",
		title:    "OpenCode Snapshots",
		path:     func() string { return filepath.Join(opencodeDataRoot(), "snapshot") },
		category: core.CategorySessionStore,
		subdir:   filepath.Join("opencode", "snapshot"),
		warning:  "OpenCode keeps the file snapshots behind undo here.",
	},
	{
		id:       "opencode.storage",
		title:    "OpenCode Storage",
		path:     func() string { return filepath.Join(opencodeDataRoot(), "storage") },
		category: core.CategorySessionStore,
		subdir:   filepath.Join("opencode", "storage"),
		warning:  "OpenCode keeps the records of each message here.",
	},
}

// NewClaudeProvider moves the conversation history of Claude Code.
func NewClaudeProvider() *linkedProvider {
	return &linkedProvider{tool: "claude", lock: []string{"claude"}, rows: claudeFolders}
}

// NewCodexProvider moves the conversations that Codex CLI wrote.
func NewCodexProvider() *linkedProvider {
	return &linkedProvider{tool: "codex", lock: []string{"codex"}, rows: codexFolders}
}

// NewOpenCodeProvider moves the conversation store of OpenCode.
func NewOpenCodeProvider() *linkedProvider {
	return &linkedProvider{tool: "opencode", lock: []string{"opencode"}, rows: opencodeRows}
}
