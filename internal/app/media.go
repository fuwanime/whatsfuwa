package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

const maxMediaBytes = 64 << 20

type mediaDownloader func(context.Context, whatsmeow.DownloadableMessage) ([]byte, error)

func mediaContent(message *waE2E.Message) (whatsmeow.DownloadableMessage, string, string) {
	if message == nil {
		return nil, "", ""
	}
	switch content := message.GetImageMessage(); content != nil {
	case true:
		return content, "image", content.GetMimetype()
	}
	switch content := message.GetVideoMessage(); content != nil {
	case true:
		return content, "video", content.GetMimetype()
	}
	switch content := message.GetAudioMessage(); content != nil {
	case true:
		return content, "audio", content.GetMimetype()
	}
	switch content := message.GetDocumentMessage(); content != nil {
	case true:
		return content, "document", content.GetMimetype()
	}
	switch content := message.GetStickerMessage(); content != nil {
	case true:
		return content, "sticker", content.GetMimetype()
	}
	return nil, "", ""
}

func (w *WhatsAppClient) DownloadMedia(ctx context.Context, message ChatMessage) (string, error) {
	if message.MediaPath != "" {
		if path, err := downloadMedia(ctx, w.store, message, nil); err == nil {
			return path, nil
		}
	}
	if !w.Client.IsConnected() {
		return "", fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}
	return downloadMedia(ctx, w.store, message, w.Client.Download)
}

func downloadMedia(ctx context.Context, store *ChatStore, message ChatMessage, download mediaDownloader) (string, error) {
	if message.MediaPath != "" {
		if info, err := os.Stat(message.MediaPath); err == nil && info.Mode().IsRegular() {
			return message.MediaPath, nil
		}
	}
	if len(message.RawMessage) == 0 {
		return "", fmt.Errorf("metadata media tidak tersedia untuk pesan ini")
	}
	if download == nil {
		return "", fmt.Errorf("pengunduh media tidak tersedia")
	}
	var raw waE2E.Message
	if err := proto.Unmarshal(message.RawMessage, &raw); err != nil {
		return "", fmt.Errorf("membaca metadata media: %w", err)
	}
	downloadable, _, mimeType := mediaContent(&raw)
	if downloadable == nil {
		return "", fmt.Errorf("pesan ini bukan media yang dapat diunduh")
	}
	data, err := download(ctx, downloadable)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxMediaBytes {
		return "", fmt.Errorf("media terlalu besar; batas %d MiB", maxMediaBytes>>20)
	}
	if store == nil {
		return "", fmt.Errorf("penyimpanan media tidak tersedia")
	}
	mediaDir := filepath.Join(filepath.Dir(store.path), "media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return "", err
	}
	name := mediaFilename(message.ChatJID, message.ID, mimeType)
	path := filepath.Join(mediaDir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := store.SetMediaPath(message.ChatJID, message.ID, path, mimeType); err != nil {
		return path, fmt.Errorf("media terunduh, tetapi gagal menyimpan metadata: %w", err)
	}
	return path, nil
}

func mediaFilename(chatJID, messageID, mimeType string) string {
	digest := sha256.Sum256([]byte(chatJID + "\x00" + messageID))
	return hex.EncodeToString(digest[:8]) + mediaExtension(mimeType)
}

func mediaExtension(mimeType string) string {
	mimeType, _, _ = mime.ParseMediaType(mimeType)
	extensions := map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp",
		"video/mp4": ".mp4", "video/webm": ".webm", "audio/ogg": ".ogg", "audio/mpeg": ".mp3",
		"audio/mp4": ".m4a", "audio/aac": ".aac", "application/pdf": ".pdf",
		"application/zip": ".zip", "text/plain": ".txt",
	}
	if extension := extensions[strings.ToLower(mimeType)]; extension != "" {
		return extension
	}
	return ".bin"
}
