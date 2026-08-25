package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirMightHaveChangedDetectsNewTheme(t *testing.T) {
	dir := t.TempDir()
	dirSnapshots = map[string]string{}
	t.Cleanup(func() { dirSnapshots = map[string]string{} })

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

func TestDirMightHaveChangedDetectsOlderThemeDeletion(t *testing.T) {
	dir := t.TempDir()
	dirSnapshots = map[string]string{}
	t.Cleanup(func() { dirSnapshots = map[string]string{} })

	older := filepath.Join(dir, "older.json")
	newer := filepath.Join(dir, "newer.json")
	for _, path := range []string{older, newer} {
		if err := os.WriteFile(path, []byte(`{"background":"dark"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Second)
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, future, future); err != nil {
		t.Fatal(err)
	}
	if dirMightHaveChanged(dir) {
		t.Fatal("first scan must establish the baseline")
	}
	if err := os.Remove(older); err != nil {
		t.Fatal(err)
	}
	if !dirMightHaveChanged(dir) {
		t.Fatal("deleting a non-newest theme must change the directory snapshot")
	}
}
