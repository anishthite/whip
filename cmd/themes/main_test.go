package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirMightHaveChangedDetectsNewTheme(t *testing.T) {
	dir := t.TempDir()
	dirMtimes = map[string]time.Time{}
	t.Cleanup(func() { dirMtimes = map[string]time.Time{} })

	if dirMightHaveChanged(dir) {
		t.Fatal("first scan must establish the baseline")
	}
	path := filepath.Join(dir, "nord.json")
	if err := os.WriteFile(path, []byte(`{"background":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if !dirMightHaveChanged(dir) {
		t.Fatal("new theme file should change the directory snapshot")
	}
	if dirMightHaveChanged(dir) {
		t.Fatal("unchanged directory should not trigger a second reload")
	}
}
