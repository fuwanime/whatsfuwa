package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fuwanime/whatsfuwa/internal/app"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update":
			if err := runUpdate(); err != nil {
				log.Fatal(err)
			}
			return
		case "version", "-v", "--version":
			printVersion()
			return
		case "help", "-h", "--help":
			printHelp()
			return
		default:
			fmt.Fprintf(os.Stderr, "perintah tidak dikenal: %q\n\n", os.Args[1])
			printHelp()
			os.Exit(2)
		}
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func dataDirectory() (string, error) {
	if dir := os.Getenv("WHATSFUWA_DATA_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("menemukan folder konfigurasi: %w", err)
	}
	return filepath.Join(base, "whatsfuwa"), nil
}

func run() error {
	dataDir, err := dataDirectory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("buat folder data: %w", err)
	}
	_ = os.Chmod(dataDir, 0o700)

	dbPath := filepath.Join(dataDir, "session.db")
	// modernc.org/sqlite registers the pure-Go driver as "sqlite". whatsmeow
	// still receives its own SQLite dialect name ("sqlite3") for SQL generation.
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("buka database sesi: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := container.Upgrade(ctx); err != nil {
		return fmt.Errorf("inisialisasi database sesi: %w", err)
	}
	defer container.Close()
	_ = os.Chmod(dbPath, 0o600)

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("memuat sesi WhatsApp: %w", err)
	}

	chatStore, err := app.OpenChatStore(filepath.Join(dataDir, "chats.json"))
	if err != nil {
		return fmt.Errorf("membuka cache chat: %w", err)
	}

	clientLog := waLog.Stdout("WhatsFuwa", "WARN", true)
	waClient := app.NewWhatsAppClient(device, clientLog, chatStore)
	waClient.Client.EnableAutoReconnect = true
	waClient.Client.InitialAutoReconnect = true
	// Jangan unduh arsip chat penuh saat login. Ini menghemat bandwidth,
	// ruang penyimpanan, dan lonjakan memori di laptop lama.
	waClient.Client.ManualHistorySyncDownload = true
	// Minta ulang pesan yang gagal diterima dari perangkat utama bila diperlukan.
	waClient.Client.AutomaticMessageRerequestFromPhone = true

	ctxSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := waClient.Connect(ctxSignal); err != nil {
		return err
	}

	ui := app.NewUI(waClient, chatStore)
	if err := ui.Run(ctxSignal); err != nil {
		return err
	}

	waClient.Disconnect()
	if err := chatStore.Save(); err != nil {
		return fmt.Errorf("menyimpan cache chat: %w", err)
	}
	return nil
}
