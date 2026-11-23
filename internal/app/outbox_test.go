package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestOutboxSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	store, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	entry, err := store.EnqueueText(jid, "offline message", &ChatMessage{ID: "reply", ChatJID: jid.String(), Text: "quoted"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID == "" {
		t.Fatal("queued message has no stable ID")
	}
	reloaded, err := OpenChatStore(path)
	if err != nil {
		t.Fatal(err)
	}
	entries := reloaded.Outbox()
	if len(entries) != 1 || entries[0].ID != entry.ID || entries[0].Text != "offline message" || entries[0].Reply == nil || entries[0].Reply.ID != "reply" {
		t.Fatalf("outbox did not survive restart: %#v", entries)
	}
}

func TestRetryOutboxUsesStableIDAndRemovesAccepted(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	entry, err := store.EnqueueText(jid, "offline message", nil)
	if err != nil {
		t.Fatal(err)
	}
	factory := func(id types.MessageID) sendMessageFunc {
		if id != types.MessageID(entry.ID) {
			t.Fatalf("retry changed message ID: got %q want %q", id, entry.ID)
		}
		return func(_ context.Context, _ types.JID, message *waE2E.Message) (whatsmeow.SendResponse, error) {
			if message.GetConversation() != "offline message" {
				t.Fatalf("wrong queued text: %q", message.GetConversation())
			}
			return whatsmeow.SendResponse{ID: id, Timestamp: time.Now()}, nil
		}
	}
	sent, err := retryOutbox(context.Background(), store, factory)
	if sent != 1 || err != nil {
		t.Fatalf("retry failed: sent=%d err=%v", sent, err)
	}
	if len(store.Outbox()) != 0 {
		t.Fatalf("accepted message remained queued: %#v", store.Outbox())
	}
	messages := store.Chats()[0].Messages
	if len(messages) != 1 || messages[0].ID != entry.ID {
		t.Fatalf("stable ID not used in cache: %#v", messages)
	}
}

func TestRetryOutboxKeepsAmbiguousFailureForManualReview(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	if _, err := store.EnqueueText(jid, "offline message", nil); err != nil {
		t.Fatal(err)
	}
	calls := 0
	factory := func(id types.MessageID) sendMessageFunc {
		return func(context.Context, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error) {
			calls++
			return whatsmeow.SendResponse{}, errors.New("timeout: acceptance unknown")
		}
	}
	sent, err := retryOutbox(context.Background(), store, factory)
	if sent != 0 || err == nil || calls != 1 {
		t.Fatalf("ambiguous retry was not conservative: sent=%d err=%v calls=%d", sent, err, calls)
	}
	entries := store.Outbox()
	if len(entries) != 1 || entries[0].Attempts != 1 || !strings.Contains(entries[0].LastError, "acceptance unknown") {
		t.Fatalf("failed retry was not retained for review: %#v", entries)
	}
	if len(store.Chats()) != 0 {
		t.Fatal("failed retry was cached as sent")
	}
}

func TestQueuedSendDoesNotRestoreComposer(t *testing.T) {
	model := tuiModel{}
	next, _ := model.Update(sendResultMsg{jid: "chat", text: "offline", err: ErrOutboxQueued})
	model = next.(tuiModel)
	if model.compose != "" || !strings.Contains(strings.ToLower(model.toast), "outbox") {
		t.Fatalf("queued send was treated as a failure: compose=%q toast=%q", model.compose, model.toast)
	}
}
