package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestDeliverTextSeparatesSendAndPersistenceResults(t *testing.T) {
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	response := whatsmeow.SendResponse{ID: "message-id", Timestamp: time.Now()}

	t.Run("accepted but cache failed", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		store := &ChatStore{path: filepath.Join(blocker, "chats.json"), chats: make(map[string]*Chat)}
		send := func(context.Context, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error) {
			return response, nil
		}
		sent, err := deliverText(context.Background(), store, jid, "hello", nil, send)
		if !sent || err == nil {
			t.Fatalf("accepted send with failed cache = sent %v, err %v", sent, err)
		}
	})

	t.Run("send rejected", func(t *testing.T) {
		sendErr := errors.New("offline")
		send := func(context.Context, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error) {
			return whatsmeow.SendResponse{}, sendErr
		}
		sent, err := deliverText(context.Background(), nil, jid, "hello", nil, send)
		if sent || !errors.Is(err, sendErr) {
			t.Fatalf("rejected send = sent %v, err %v", sent, err)
		}
	})
}

func TestSendResultRestoresOnlyUnsentDraft(t *testing.T) {
	reply := &ChatMessage{ID: "reply", ChatJID: "chat", Text: "original"}
	for _, test := range []struct {
		name        string
		selectedJID string
		compose     string
		wantCompose string
		wantReply   bool
	}{
		{name: "empty composer", selectedJID: "chat", wantCompose: "unsent", wantReply: true},
		{name: "newer composer text", selectedJID: "chat", compose: "newer", wantCompose: "unsent\nnewer", wantReply: true},
		{name: "different selected chat", selectedJID: "other", compose: "other draft", wantCompose: "other draft"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
			if err != nil {
				t.Fatal(err)
			}
			store.AddChat("chat", "")
			store.AddChat("other", "")
			model := tuiModel{ui: &UI{store: store}, chats: store.Chats(), selectedJID: test.selectedJID, compose: test.compose}
			next, _ := model.Update(sendResultMsg{jid: "chat", text: "unsent", reply: reply, err: errors.New("offline")})
			model = next.(tuiModel)
			if model.compose != test.wantCompose || (model.reply != nil) != test.wantReply {
				t.Fatalf("restored draft = %q, reply %#v; want %q, reply present %v", model.compose, model.reply, test.wantCompose, test.wantReply)
			}
			if test.selectedJID == "other" {
				model.selectedJID = "chat"
				model.refreshChats()
				if model.compose != "unsent\nother draft" || model.reply == nil {
					t.Fatalf("pending draft was not restored on returning to chat: compose=%q reply=%#v", model.compose, model.reply)
				}
				before := model.compose
				model.refreshChats()
				if model.compose != before {
					t.Fatalf("pending draft was restored twice: %q", model.compose)
				}
			}
		})
	}
}

func TestNewestFailedSendControlsRecoveredReply(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "active"
		if pending {
			name = "pending"
		}
		t.Run(name, func(t *testing.T) {
			store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
			if err != nil {
				t.Fatal(err)
			}
			store.AddChat("chat", "")
			store.AddChat("other", "")
			selected := "chat"
			if pending {
				selected = "other"
			}
			model := tuiModel{ui: &UI{store: store}, chats: store.Chats(), selectedJID: selected}
			firstReply := &ChatMessage{ID: "first-reply", ChatJID: "chat"}
			secondReply := &ChatMessage{ID: "second-reply", ChatJID: "chat"}
			next, _ := model.Update(sendResultMsg{jid: "chat", text: "first", reply: firstReply, err: errors.New("offline")})
			model = next.(tuiModel)
			next, _ = model.Update(sendResultMsg{jid: "chat", text: "second", reply: secondReply, err: errors.New("offline")})
			model = next.(tuiModel)
			model.selectedJID = "chat"
			model.refreshChats()
			if model.compose != "second\nfirst" || model.reply == nil || model.reply.ID != "second-reply" {
				t.Fatalf("wrong recovered draft or reply: compose=%q reply=%#v", model.compose, model.reply)
			}
		})
	}
}

func TestAcceptedSendReportsCacheWarning(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.AddChat("chat", "")
	model := tuiModel{ui: &UI{store: store}, chats: store.Chats(), selectedJID: "chat"}
	next, _ := model.Update(sendResultMsg{jid: "chat", text: "sent", sent: true, err: errors.New("disk full")})
	model = next.(tuiModel)
	toast := strings.ToLower(model.toast)
	if !strings.Contains(toast, "terkirim") || !strings.Contains(toast, "cache") || strings.Contains(toast, "gagal mengirim") {
		t.Fatalf("accepted send was not distinguished from failure: %q", model.toast)
	}
	if model.compose != "" {
		t.Fatalf("sent draft was restored: %q", model.compose)
	}
}
