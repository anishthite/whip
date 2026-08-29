package main

import (
	"errors"
	"strings"
)

const upUsage = "usage: whip up <prompt>"

// upCLI turns every shell argument after `whip up` into one agent prompt.
func upCLI(args []string) error {
	prompt := strings.TrimSpace(strings.Join(args, " "))
	if prompt == "" {
		return errors.New(upUsage)
	}

	// The terminator makes a prompt such as "--format json" data rather than
	// an option to the underlying `whip run` command.
	return runCLI([]string{"--", prompt})
}
