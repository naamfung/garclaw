package main

import (
	"log"
	"os"
	"strings"

	"github.com/toon-format/toon-go"
)

// AliasEntry represents a single command alias.
type AliasEntry struct {
	Name    string `toon:"Name"`
	Command string `toon:"Command"`
}

// ToolsAliasConfig is the top-level config structure for tools.toon.
type ToolsAliasConfig struct {
	Aliases []AliasEntry `toon:"Aliases"`
}

// globalToolsAliases stores the loaded alias map.
var globalToolsAliases map[string]string

// LoadToolsAliases loads alias definitions from a TOON file and returns a map
// from alias name to expanded command.
func LoadToolsAliases(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	var config ToolsAliasConfig
	if err := toon.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	aliases := make(map[string]string)
	for _, entry := range config.Aliases {
		if entry.Name != "" && entry.Command != "" {
			aliases[entry.Name] = entry.Command
		}
	}

	log.Printf("Loaded %d tool alias(es) from %s", len(aliases), path)
	return aliases, nil
}

// ExpandAlias checks if the first word of a command matches an alias and expands it.
// If no alias matches, the original command is returned unchanged.
func ExpandAlias(command string, aliases map[string]string) string {
	if aliases == nil || len(aliases) == 0 {
		return command
	}

	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return command
	}

	// Extract the first word
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return command
	}

	firstWord := fields[0]
	if expanded, ok := aliases[firstWord]; ok {
		// Replace the first word with the alias command, preserving the rest of the arguments
		if len(fields) > 1 {
			return expanded + " " + strings.Join(fields[1:], " ")
		}
		return expanded
	}

	return command
}
