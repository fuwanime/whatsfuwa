package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestHistoryRequestUsesOldestMessage(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	oldest := time.Now().Add(-time.Hour)
	for _, message := range []ChatMessage{
		{ID: "old", ChatJID: jid.String(), Timestamp: oldest},
		{ID: "new", ChatJID: jid.String(), Timestamp: time.Now()},
	} {
		if err := store.AddMessage(message, ""); err != nil {
			t.Fatal(err)
		}
	}
	var gotInfo *types.MessageInfo
	build := func(info *types.MessageInfo, count int) *waE2E.Message {
		gotInfo = info
		if count != 50 {
			t.Fatalf("history count = %d, want 50", count)
		}
		return &waE2E.Message{Conversation: proto.String("history-request")}
	}
	sent := false
	send := func(_ context.Context, message *waE2E.Message) (whatsmeow.SendResponse, error) {
		sent = true
		if message.GetConversation() != "history-request" {
			t.Fatalf("wrong history request: %#v", message)
		}
		return whatsmeow.SendResponse{}, nil
	}
	if err := requestHistory(context.Background(), store, jid, build, send); err != nil {
		t.Fatal(err)
	}
	if !sent || gotInfo == nil || gotInfo.ID != "old" || gotInfo.Chat != jid || !gotInfo.Timestamp.Equal(oldest) {
		t.Fatalf("request did not use oldest message: sent=%v info=%#v", sent, gotInfo)
	}
}

func TestHistorySyncIngestsAndDeduplicates(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	if err := store.AddMessage(ChatMessage{ID: "new", ChatJID: jid.String(), Text: "new", Timestamp: time.Now()}, ""); err != nil {
		t.Fatal(err)
	}
	raw := &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("old")}}
	data := &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{{
		ID: proto.String(jid.String()), Messages: []*waHistorySync.HistorySyncMsg{{Message: raw}},
	}}}
	parse := func(chatJID types.JID, webMessage *waWeb.WebMessageInfo) (*events.Message, error) {
		event := messageEvent(chatJID, &waE2E.Message{Conversation: proto.String("old")})
		event.Info.ID = types.MessageID(webMessage.GetKey().GetID())
		event.Info.Timestamp = time.Now().Add(-time.Hour)
		event.SourceWebMsg = webMessage
		return event, nil
	}
	client := &WhatsAppClient{store: store}
	for i := 0; i < 2; i++ {
		if err := client.handleHistorySync(data, parse); err != nil {
			t.Fatal(err)
		}
	}
	chat := store.Chats()[0]
	if len(chat.Messages) != 2 || chat.Messages[0].ID != "old" || chat.Messages[1].ID != "new" {
		t.Fatalf("history was not ingested chronologically: %#v", chat.Messages)
	}
	if chat.Unread != 0 {
		t.Fatalf("history incorrectly became unread: %d", chat.Unread)
	}
}
