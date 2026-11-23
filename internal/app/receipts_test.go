package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestIncomingMessageCountsUnreadOnce(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &WhatsAppClient{store: store}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	event := messageEvent(jid, &waE2E.Message{Conversation: proto.String("incoming")})
	client.handleEvent(event)
	client.handleEvent(event)
	chat := store.Chats()[0]
	if chat.Unread != 1 || !chat.Messages[0].Unread {
		t.Fatalf("incoming unread was not counted once: %#v", chat)
	}
}

func TestReceiptUpdatesDeliveryWithoutDowngrade(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	if err := store.AddMessage(ChatMessage{ID: "out", ChatJID: jid.String(), FromMe: true, Delivery: DeliverySent, Timestamp: time.Now()}, ""); err != nil {
		t.Fatal(err)
	}
	client := &WhatsAppClient{store: store}
	for _, receiptType := range []types.ReceiptType{types.ReceiptTypeRead, types.ReceiptTypeDelivered} {
		client.handleEvent(&events.Receipt{
			MessageSource: types.MessageSource{Chat: jid}, MessageIDs: []types.MessageID{"out"}, Type: receiptType, Timestamp: time.Now(),
		})
	}
	if got := store.Chats()[0].Messages[0].Delivery; got != DeliveryRead {
		t.Fatalf("delivery was downgraded or not updated: %v", got)
	}
	view := strings.Join((tuiModel{chats: store.Chats(), selectedJID: jid.String()}).renderConversation(60, 10), "\n")
	if !strings.Contains(view, "dibaca") {
		t.Fatalf("read status is not visible: %q", view)
	}
}

func TestReadSelfClearsIncomingUnread(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	if err := store.AddMessage(ChatMessage{ID: "in", ChatJID: jid.String(), Unread: true, Timestamp: time.Now()}, ""); err != nil {
		t.Fatal(err)
	}
	client := &WhatsAppClient{store: store}
	client.handleEvent(&events.Receipt{
		MessageSource: types.MessageSource{Chat: jid}, MessageIDs: []types.MessageID{"in"}, Type: types.ReceiptTypeReadSelf, Timestamp: time.Now(),
	})
	chat := store.Chats()[0]
	if chat.Unread != 0 || chat.Messages[0].Unread {
		t.Fatalf("read-self did not clear incoming unread: %#v", chat)
	}
}

func TestMarkChatReadGroupsBySender(t *testing.T) {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	chatJID := types.NewJID("120363", types.GroupServer)
	sender := types.NewJID("628111", types.DefaultUserServer)
	if err := store.AddMessage(ChatMessage{ID: "in", ChatJID: chatJID.String(), SenderJID: sender.String(), Unread: true, Timestamp: time.Now()}, ""); err != nil {
		t.Fatal(err)
	}
	called := false
	mark := func(_ context.Context, ids []types.MessageID, _ time.Time, chat, gotSender types.JID) error {
		called = true
		if len(ids) != 1 || ids[0] != "in" || chat != chatJID || gotSender != sender {
			t.Fatalf("wrong read receipt: ids=%v chat=%v sender=%v", ids, chat, gotSender)
		}
		return nil
	}
	if err := markChatRead(context.Background(), store, chatJID, mark); err != nil {
		t.Fatal(err)
	}
	if !called || store.Chats()[0].Unread != 0 || store.Chats()[0].Messages[0].Unread {
		t.Fatalf("mark read did not clear store: called=%v chat=%#v", called, store.Chats()[0])
	}
}
