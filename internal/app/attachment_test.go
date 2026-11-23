package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func fakeUploadResponse() whatsmeow.UploadResponse {
	return whatsmeow.UploadResponse{
		URL: "https://upload.example", DirectPath: "/media/path", MediaKey: []byte("key"),
		FileSHA256: []byte("plain-hash"), FileEncSHA256: []byte("enc-hash"), FileLength: 2,
	}
}

func TestSendAttachmentBuildsImageMessage(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	path := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(path, testPNG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	uploadResponse := fakeUploadResponse()
	upload := func(_ context.Context, data []byte, mediaType whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		if mediaType != whatsmeow.MediaImage || len(data) == 0 {
			t.Fatalf("wrong upload: type=%v bytes=%d", mediaType, len(data))
		}
		return uploadResponse, nil
	}
	send := func(_ context.Context, _ types.JID, message *waE2E.Message) (whatsmeow.SendResponse, error) {
		image := message.GetImageMessage()
		if image == nil || image.GetMimetype() != "image/png" || image.GetCaption() != "caption" || image.GetURL() != uploadResponse.URL || image.GetDirectPath() != uploadResponse.DirectPath {
			t.Fatalf("wrong image message: %#v", image)
		}
		return whatsmeow.SendResponse{ID: "sent-image", Timestamp: time.Now()}, nil
	}
	sent, err := sendAttachment(context.Background(), store, jid, path, "caption", nil, upload, send)
	if !sent || err != nil {
		t.Fatalf("attachment failed: sent=%v err=%v", sent, err)
	}
	messages := store.Chats()[0].Messages
	if len(messages) != 1 || messages[0].ID != "sent-image" || messages[0].MediaType != "image" || messages[0].MediaPath != path || len(messages[0].RawMessage) == 0 {
		t.Fatalf("sent attachment was not cached correctly: %#v", messages)
	}
	if !strings.Contains(messages[0].Text, "caption") {
		t.Fatalf("caption missing from preview: %q", messages[0].Text)
	}
}

func TestSendAttachmentFallsBackToDocument(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	path := filepath.Join(t.TempDir(), "payload.unknown")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	upload := func(_ context.Context, _ []byte, mediaType whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		if mediaType != whatsmeow.MediaDocument {
			t.Fatalf("unknown file type = %v, want document", mediaType)
		}
		return fakeUploadResponse(), nil
	}
	send := func(_ context.Context, _ types.JID, message *waE2E.Message) (whatsmeow.SendResponse, error) {
		document := message.GetDocumentMessage()
		if document == nil || document.GetFileName() != "payload.unknown" {
			t.Fatalf("document fallback wrong: %#v", document)
		}
		return whatsmeow.SendResponse{ID: "doc", Timestamp: time.Now()}, nil
	}
	if sent, err := sendAttachment(context.Background(), store, jid, path, "", nil, upload, send); !sent || err != nil {
		t.Fatalf("document send failed: sent=%v err=%v", sent, err)
	}
}

func TestSendAttachmentRejectsLargeFileBeforeUpload(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "large.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxMediaBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	called := false
	upload := func(context.Context, []byte, whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		called = true
		return fakeUploadResponse(), nil
	}
	sent, err := sendAttachment(context.Background(), store, types.NewJID("628123456789", types.DefaultUserServer), path, "", nil, upload, nil)
	if sent || err == nil || !strings.Contains(err.Error(), "terlalu besar") || called {
		t.Fatalf("large file was uploaded: sent=%v err=%v called=%v", sent, err, called)
	}
}
