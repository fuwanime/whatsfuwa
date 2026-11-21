package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

const modulePathLine = "module github.com/fuwanime/whatsfuwa"

// runUpdate pulls the latest source with git and rebuilds the binary in place.
// It needs the source checkout, which is found via WHATSFUWA_SRC, the current
// folder, or the folder that holds the executable.
func runUpdate() error {
	src, err := findSourceDir()
	if err != nil {
		return err
	}

	fmt.Printf("Mengambil pembaruan dari %s...\n", src)
	if err := runCmd(src, "git", "pull", "--ff-only"); err != nil {
		return fmt.Errorf("git pull gagal (pastikan tidak ada perubahan lokal yang bentrok): %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("menemukan lokasi program: %w", err)
	}
	tmp := exe + ".new"

	fmt.Println("Membangun ulang WhatsFuwa...")
	if err := runCmd(src, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", tmp, "./cmd/whatsfuwa"); err != nil {
		return fmt.Errorf("build gagal: %w", err)
	}

	// Windows cannot replace a running executable, so leave the new build
	// next to the old one and let the user swap them after closing WhatsFuwa.
	if runtime.GOOS == "windows" {
		fmt.Printf("Build baru disimpan di %s. Tutup WhatsFuwa, lalu ganti file lama dengan file ini.\n", tmp)
		return nil
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("mengganti program: %w", err)
	}
	fmt.Println("WhatsFuwa berhasil diperbarui. Jalankan lagi untuk memakai versi baru.")
	return nil
}

func findSourceDir() (string, error) {
	var candidates []string
	if env := os.Getenv("WHATSFUWA_SRC"); env != "" {
		candidates = append(candidates, env)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	for _, dir := range candidates {
		if isSourceDir(dir) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("folder source WhatsFuwa tidak ditemukan; jalankan dari folder repositori atau atur WHATSFUWA_SRC")
}

// isSourceDir reports whether dir is a git checkout of this module.
func isSourceDir(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil || !strings.Contains(string(data), modulePathLine) {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printVersion() {
	fmt.Println("WhatsFuwa", buildVersion())
}

// buildVersion returns the git commit the binary was built from, or "dev" when
// build information is not available.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

func printHelp() {
	fmt.Print(`WhatsFuwa — klien WhatsApp di terminal.

Penggunaan:
  whatsfuwa            Buka aplikasi
  whatsfuwa update     Ambil pembaruan dari git dan bangun ulang
  whatsfuwa version    Tampilkan versi
  whatsfuwa help       Tampilkan bantuan ini
`)
}
