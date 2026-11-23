package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/fuwanime/whatsfuwa/internal/app"
	"golang.org/x/term"
)

func runMedia(args []string, wait bool) error {
	if len(args) != 1 {
		return fmt.Errorf("penggunaan: whatsfuwa media <path-gambar>")
	}
	info, err := os.Stat(args[0])
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path bukan file gambar")
	}
	protocol := app.DetectImageProtocol(os.Getenv)
	if protocol == app.ImageNone {
		return fmt.Errorf("terminal tidak dikenali sebagai Kitty/Sixel; atur WHATSFUWA_IMAGE_PROTOCOL=kitty atau sixel, atau buka file secara eksternal: %s", args[0])
	}
	columns, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		columns, rows = 80, 24
	}
	if err := app.WriteImagePreview(os.Stdout, args[0], protocol, columns, rows); err != nil {
		return err
	}
	fmt.Print("\r\n")
	if wait {
		fmt.Println("Tekan Enter untuk kembali ke WhatsFuwa...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return nil
}
