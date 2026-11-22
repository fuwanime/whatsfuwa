package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestChatStorePersistsAndSortsChats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	store, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := store.AddMessage(ChatMessage{
		ID: "message-1", ChatJID: "628111@s.whatsapp.net", Text: "halo", Timestamp: now,
	}, "Rani"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(ChatMessage{
		ID: "message-2", ChatJID: "628222@s.whatsapp.net", Text: "hai", Timestamp: now.Add(time.Minute),
	}, "Budi"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	chats := reloaded.Chats()
	if len(chats) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(chats))
	}
	if chats[0].JID != "628222@s.whatsapp.net" {
		t.Fatalf("expected newest chat first, got %s", chats[0].JID)
	}
	if chats[1].Title != "Rani" || len(chats[1].Messages) != 1 {
		t.Fatalf("chat cache was not restored correctly: %#v", chats[1])
	}
}

func TestChatStoreBreaksTimestampTiesDeterministically(t *testing.T) {
	now := time.Now()
	store := &ChatStore{chats: make(map[string]*Chat)}
	for i := 0; i < 20; i++ {
		jid := string(rune('a' + i))
		store.chats[jid] = &Chat{JID: jid, UpdatedAt: now}
	}
	for attempt := 0; attempt < 5; attempt++ {
		chats := store.Chats()
		for i, chat := range chats {
			if want := string(rune('a' + i)); chat.JID != want {
				t.Fatalf("equal timestamps produced unstable order at attempt %d index %d: got %q, want %q", attempt, i, chat.JID, want)
			}
		}
	}
}

func TestChatStoreIgnoresDuplicateMessageIDs(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	message := ChatMessage{ID: "same-id", ChatJID: "628111@s.whatsapp.net", Text: "one"}
	if err := store.AddMessage(message, "Rani"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(message, "Rani"); err != nil {
		t.Fatal(err)
	}
	chats := store.Chats()
	if len(chats) != 1 || len(chats[0].Messages) != 1 {
		t.Fatalf("duplicate message was stored: %#v", chats)
	}
}

func TestChatStoreBoundsMessagesAndChats(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxMessages+5; i++ {
		message := ChatMessage{
			ID:        string(rune('a' + i)),
			ChatJID:   "628111@s.whatsapp.net",
			Text:      "message",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := store.AddMessage(message, "Rani"); err != nil {
			t.Fatal(err)
		}
	}
	chat := store.Chats()[0]
	if len(chat.Messages) != maxMessages {
		t.Fatalf("expected at most %d messages, got %d", maxMessages, len(chat.Messages))
	}

	for i := 0; i < maxChats+5; i++ {
		store.AddChat("contact-"+string(rune('a'+i)), "Contact")
	}
	if got := len(store.Chats()); got != maxChats {
		t.Fatalf("expected at most %d chats, got %d", maxChats, got)
	}
}

func TestChatStoreTruncatesTextWithoutBreakingUTF8(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("界", maxMessageText+5)
	if err := store.AddMessage(ChatMessage{
		ID: "unicode-message", ChatJID: "628111@s.whatsapp.net", Text: text,
	}, "Rani"); err != nil {
		t.Fatal(err)
	}
	got := store.Chats()[0].Messages[0].Text
	if !utf8.ValidString(got) {
		t.Fatalf("stored text is not valid UTF-8: %q", got)
	}
	if got != strings.Repeat("界", maxMessageText)+"…" {
		t.Fatalf("text was not safely truncated: got %d runes", len([]rune(got)))
	}
}
