package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func updateFixture(t *testing.T) (*WhatsAppClient, *ChatStore, types.JID) {
	t.Helper()
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	if err := store.AddMessage(ChatMessage{ID: "m1", ChatJID: jid.String(), SenderJID: jid.String(), Text: "old", Timestamp: time.Now()}, ""); err != nil {
		t.Fatal(err)
	}
	return &WhatsAppClient{store: store}, store, jid
}

func messageEvent(jid types.JID, message *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: jid, Sender: jid},
			ID:            "m1", Timestamp: time.Now(),
		},
		Message: message,
	}
}

func TestEditUpdatesExistingMessage(t *testing.T) {
	client, store, jid := updateFixture(t)
	event := messageEvent(jid, &waE2E.Message{Conversation: proto.String("new text")})
	event.IsEdit = true
	client.handleEvent(event)
	messages := store.Chats()[0].Messages
	if len(messages) != 1 || messages[0].Text != "new text" || !messages[0].Edited {
		t.Fatalf("edit was not applied to original message: %#v", messages)
	}
}

func TestReactionUpdatesPerSender(t *testing.T) {
	client, store, jid := updateFixture(t)
	for _, reaction := range []struct {
		sender string
		emoji  string
	}{
		{"sender-one@s.whatsapp.net", "🔥"},
		{"sender-two@s.whatsapp.net", "❤️"},
		{"sender-one@s.whatsapp.net", ""},
	} {
		sender, _ := types.ParseJID(reaction.sender)
		event := messageEvent(jid, &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
			Key:  &waCommon.MessageKey{ID: proto.String("m1"), RemoteJID: proto.String(jid.String())},
			Text: proto.String(reaction.emoji),
		}})
		event.Info.Sender = sender
		client.handleEvent(event)
	}
	message := store.Chats()[0].Messages[0]
	if len(message.Reactions) != 1 || message.Reactions["sender-two@s.whatsapp.net"] != "❤️" {
		t.Fatalf("reactions were not merged per sender: %#v", message.Reactions)
	}
	if view := strings.Join((tuiModel{chats: store.Chats(), selectedJID: jid.String()}).renderConversation(60, 10), "\n"); !strings.Contains(view, "❤️") {
		t.Fatalf("reaction is not rendered: %q", view)
	}
}

func TestRevokeLeavesTombstone(t *testing.T) {
	client, store, jid := updateFixture(t)
	client.handleEvent(messageEvent(jid, &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("m1"), RemoteJID: proto.String(jid.String())},
	}}))
	message := store.Chats()[0].Messages[0]
	if !message.Deleted {
		t.Fatalf("revoke did not mark tombstone: %#v", message)
	}
	if view := strings.Join((tuiModel{chats: store.Chats(), selectedJID: jid.String()}).renderConversation(60, 10), "\n"); !strings.Contains(strings.ToLower(view), "dihapus") {
		t.Fatalf("tombstone is not rendered: %q", view)
	}
}

func TestDeleteForMeLeavesTombstone(t *testing.T) {
	client, store, jid := updateFixture(t)
	client.handleEvent(&events.DeleteForMe{ChatJID: jid, MessageID: "m1", Timestamp: time.Now()})
	if !store.Chats()[0].Messages[0].Deleted {
		t.Fatal("DeleteForMe did not mark tombstone")
	}
}
