package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSourceDir(t *testing.T) {
	good := t.TempDir()
	os.WriteFile(filepath.Join(good, "go.mod"), []byte("module github.com/fuwanime/whatsfuwa\n\ngo 1.26.0\n"), 0o644)
	os.Mkdir(filepath.Join(good, ".git"), 0o755)
	if !isSourceDir(good) {
		t.Fatal("expected checkout to be recognized")
	}

	noGit := t.TempDir()
	os.WriteFile(filepath.Join(noGit, "go.mod"), []byte("module github.com/fuwanime/whatsfuwa\n"), 0o644)
	if isSourceDir(noGit) {
		t.Fatal("folder without .git must not count as a checkout")
	}

	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "go.mod"), []byte("module example.com/other\n"), 0o644)
	os.Mkdir(filepath.Join(other, ".git"), 0o755)
	if isSourceDir(other) {
		t.Fatal("different module must not be recognized")
	}
}

func TestBuildVersionWithoutVCSInfo(t *testing.T) {
	if v := buildVersion(); v == "" {
		t.Fatal("version must not be empty")
	}
}
