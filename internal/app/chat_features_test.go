package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetTitleRenamesExistingChatOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	store, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Renaming an unknown chat must not create it.
	if err := store.SetTitle("120363@g.us", "Keluarga"); err != nil {
		t.Fatal(err)
	}
	if len(store.Chats()) != 0 {
		t.Fatalf("SetTitle created a chat for an unknown JID")
	}

	if err := store.AddMessage(ChatMessage{
		ID: "group-msg", ChatJID: "120363@g.us", Text: "halo", Timestamp: time.Now(),
	}, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTitle("120363@g.us", "Keluarga"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	chats := reloaded.Chats()
	if len(chats) != 1 || chats[0].Title != "Keluarga" {
		t.Fatalf("group name was not persisted: %#v", chats)
	}
}

func TestOlderMessageDoesNotRollBackPreview(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	const jid = "628111@s.whatsapp.net"
	if err := store.AddMessage(ChatMessage{ID: "new", ChatJID: jid, Text: "terbaru", Timestamp: now}, "Rani"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(ChatMessage{ID: "old", ChatJID: jid, Text: "lama", Timestamp: now.Add(-time.Hour)}, "Rani"); err != nil {
		t.Fatal(err)
	}

	chat := store.Chats()[0]
	if chat.LastMessage != "terbaru" {
		t.Fatalf("older message overwrote preview: %q", chat.LastMessage)
	}
	if len(chat.Messages) != 2 {
		t.Fatalf("older message should still be stored in history, got %d", len(chat.Messages))
	}
	if chat.Messages[0].ID != "old" || chat.Messages[1].ID != "new" {
		t.Fatalf("messages are not chronological: %#v", chat.Messages)
	}

	model := tuiModel{chats: []Chat{chat}, selectedJID: jid}
	model.startReply()
	if model.reply == nil || model.reply.ID != "new" {
		t.Fatalf("reply did not target the newest message: %#v", model.reply)
	}
}

func TestOpenChatStoreNormalizesLegacyOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	now := time.Now()
	saved := chatFile{Version: 1, Chats: []Chat{{
		JID: "legacy", LastMessage: "stale", UpdatedAt: now.Add(-2 * time.Hour),
		Messages: []ChatMessage{
			{ID: "new", ChatJID: "legacy", Text: "new", Timestamp: now},
			{ID: "old", ChatJID: "legacy", Text: "old", Timestamp: now.Add(-time.Hour)},
		},
	}}}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	chat := store.Chats()[0]
	if chat.Messages[0].ID != "old" || chat.Messages[1].ID != "new" || chat.LastMessage != "new" || !chat.UpdatedAt.Equal(now) {
		t.Fatalf("legacy chat was not normalized: %#v", chat)
	}
}

func TestReplyFieldsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	store, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(ChatMessage{
		ID:           "reply-1",
		ChatJID:      "628111@s.whatsapp.net",
		SenderJID:    "628111@s.whatsapp.net",
		Text:         "iya",
		Timestamp:    time.Now(),
		ReplyToID:    "orig-1",
		ReplyPreview: "mau ketemu jam berapa?",
	}, "Rani"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	msg := reloaded.Chats()[0].Messages[0]
	if msg.ReplyToID != "orig-1" || msg.ReplyPreview != "mau ketemu jam berapa?" || msg.SenderJID != "628111@s.whatsapp.net" {
		t.Fatalf("reply fields were not persisted: %#v", msg)
	}
}
