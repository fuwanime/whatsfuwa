package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.White)
	img.Set(1, 0, color.Black)
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestDownloadMediaPersistsFileAndMetadata(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	client := &WhatsAppClient{store: store}
	client.handleEvent(messageEvent(jid, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Mimetype: proto.String("image/png"), DirectPath: proto.String("/remote/path"), MediaKey: []byte("key"),
	}}))
	message := store.Chats()[0].Messages[0]
	if len(message.RawMessage) == 0 || message.MediaType != "image" {
		t.Fatalf("media metadata was not stored: %#v", message)
	}
	data := testPNG(t)
	download := func(_ context.Context, downloadable whatsmeow.DownloadableMessage) ([]byte, error) {
		if _, ok := downloadable.(*waE2E.ImageMessage); !ok {
			t.Fatalf("wrong downloadable type: %T", downloadable)
		}
		return data, nil
	}
	path, err := downloadMedia(context.Background(), store, message, download)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(filepath.Dir(store.path), "media") || filepath.Ext(path) != ".png" {
		t.Fatalf("unsafe or wrong media path: %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("downloaded bytes differ: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("media is not private: %v mode=%v", err, info.Mode().Perm())
	}
	reloaded, err := OpenChatStore(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Chats()[0].Messages[0].MediaPath != path {
		t.Fatalf("media path was not persisted: %#v", reloaded.Chats()[0].Messages[0])
	}
}

func TestDownloadMediaRejectsOversizePayload(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	client := &WhatsAppClient{store: store}
	client.handleEvent(messageEvent(jid, &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		Mimetype: proto.String("application/octet-stream"), FileName: proto.String("../../outside.bin"),
	}}))
	message := store.Chats()[0].Messages[0]
	download := func(context.Context, whatsmeow.DownloadableMessage) ([]byte, error) {
		return make([]byte, maxMediaBytes+1), nil
	}
	path, err := downloadMedia(context.Background(), store, message, download)
	if err == nil || !strings.Contains(err.Error(), "terlalu besar") || path != "" {
		t.Fatalf("oversize download was accepted: path=%q err=%v", path, err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(store.path), "outside.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("remote filename escaped media directory: %v", statErr)
	}
}

func TestDownloadMediaRequiresMetadata(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = downloadMedia(context.Background(), store, ChatMessage{ID: "missing", Timestamp: time.Now()}, nil)
	if err == nil {
		t.Fatal("download without protobuf metadata succeeded")
	}
}
