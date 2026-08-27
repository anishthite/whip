package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// trusted.json records which folder paths the user has trusted (the startup
// "Do you trust the files in this folder?" dialog). Trust is per absolute
// path, like Claude Code's hasTrustDialogAccepted per project — trusting a
// folder means loopy may read its files (they feed the model) and, with
// per-command approval, execute code in it.

type trustedFile struct {
	Paths map[string]bool `json:"paths"`
}

func trustedPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trusted.json"), nil
}

// Trusted reports whether dir (absolute path) has been trusted.
func Trusted(dir string) bool {
	p, err := trustedPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var t trustedFile
	if json.Unmarshal(data, &t) != nil {
		return false
	}
	return t.Paths[dir]
}

// Trust records dir (absolute path) as trusted.
func Trust(dir string) error {
	p, err := trustedPath()
	if err != nil {
		return err
	}
	t := trustedFile{Paths: map[string]bool{}}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &t)
	}
	if t.Paths == nil {
		t.Paths = map[string]bool{}
	}
	t.Paths[dir] = true
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	LogEvent("trust.grant", dir)
	return nil
}

// TrustPromptDisabled reports whether the user has turned the folder-trust
// dialog off entirely (config.json "noTrustPrompt": true, or option 3 in
// the dialog). Untrusted folders then decline startup without asking.
func TrustPromptDisabled() bool {
	p, err := path()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var c Config
	if parseJSONC(data, &c) != nil {
		return false
	}
	return c.NoTrustPrompt
}

// DisableTrustPrompt sets "noTrustPrompt": true in config.json, preserving
// everything else. The dialog never shows again; untrusted folders decline.
func DisableTrustPrompt() error {
	p, err := path()
	if err != nil {
		return err
	}
	var c Config
	if data, err := os.ReadFile(p); err == nil {
		_ = parseJSONC(data, &c)
	}
	c.NoTrustPrompt = true
	if err := c.Save(); err != nil {
		return err
	}
	LogEvent("trust.disable-prompt", "")
	return nil
}
