package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type WhatsAppClient struct {
	Client *whatsmeow.Client
	store  *ChatStore
	logger waLog.Logger
	ready  atomic.Bool
	// namedGroups records group JIDs whose name has already been requested
	// this session, so each group is looked up at most once.
	namedGroups sync.Map
}

func NewWhatsAppClient(device *store.Device, logger waLog.Logger, chatStore *ChatStore) *WhatsAppClient {
	wa := &WhatsAppClient{
		Client: whatsmeow.NewClient(device, logger),
		store:  chatStore,
		logger: logger,
	}
	wa.Client.AddEventHandler(wa.handleEvent)
	return wa
}

func (w *WhatsAppClient) Connect(ctx context.Context) error {
	if w.Client.Store.ID == nil {
		qrChan, err := w.Client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("menyiapkan QR login: %w", err)
		}
		if err := w.Client.ConnectContext(ctx); err != nil {
			return fmt.Errorf("menghubungkan WhatsApp: %w", err)
		}

		fmt.Println("\nWhatsFuwa: scan QR ini lewat WhatsApp > Perangkat tertaut > Tautkan perangkat.")
		return waitForQRLogin(qrChan, os.Stdout)
	}

	if err := w.Client.ConnectContext(ctx); err != nil {
		return fmt.Errorf("menghubungkan sesi tersimpan: %w", err)
	}
	fmt.Println("Sesi ditemukan. Menghubungkan kembali...")
	return nil
}

func waitForQRLogin(qrChan <-chan whatsmeow.QRChannelItem, output io.Writer) error {
	succeeded := false
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, output)
		case "success":
			succeeded = true
			fmt.Fprintln(output, "\nLogin berhasil. WhatsFuwa tersambung.")
		case "timeout":
			return fmt.Errorf("QR kedaluwarsa; jalankan WhatsFuwa kembali untuk login")
		case "error":
			if evt.Error != nil {
				return fmt.Errorf("login WhatsApp gagal: %w", evt.Error)
			}
			return fmt.Errorf("login WhatsApp gagal")
		case "err-client-outdated":
			return fmt.Errorf("versi protokol WhatsApp perlu diperbarui; update WhatsFuwa")
		case "err-scanned-without-multidevice":
			return fmt.Errorf("akun belum mengaktifkan fitur multi-perangkat")
		default:
			fmt.Fprintf(output, "Status pairing: %s\n", evt.Event)
		}
	}
	if !succeeded {
		return fmt.Errorf("login WhatsApp berakhir sebelum berhasil")
	}
	return nil
}

type historyRequestBuilder func(*types.MessageInfo, int) *waE2E.Message
type peerMessageSender func(context.Context, *waE2E.Message) (whatsmeow.SendResponse, error)
type historyMessageParser func(types.JID, *waWeb.WebMessageInfo) (*events.Message, error)

func (w *WhatsAppClient) RequestHistory(ctx context.Context, chatJID types.JID) error {
	if !w.Client.IsConnected() {
		return fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}
	return requestHistory(ctx, w.store, chatJID, w.Client.BuildHistorySyncRequest, w.Client.SendPeerMessage)
}

func requestHistory(ctx context.Context, store *ChatStore, chatJID types.JID, build historyRequestBuilder, send peerMessageSender) error {
	var oldest *ChatMessage
	for _, chat := range store.Chats() {
		if chat.JID == chatJID.String() && len(chat.Messages) > 0 {
			oldest = &chat.Messages[0]
			break
		}
	}
	if oldest == nil {
		return fmt.Errorf("belum ada pesan acuan untuk meminta riwayat")
	}
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chatJID, IsFromMe: oldest.FromMe},
		ID:            types.MessageID(oldest.ID), Timestamp: oldest.Timestamp,
	}
	_, err := send(ctx, build(info, 50))
	return err
}

func (w *WhatsAppClient) handleHistorySync(data *waHistorySync.HistorySync, parse historyMessageParser) error {
	if data == nil {
		return nil
	}
	var firstErr error
	for _, conversation := range data.GetConversations() {
		chatJID, err := types.ParseJID(conversation.GetID())
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, historyMessage := range conversation.GetMessages() {
			event, err := parse(chatJID, historyMessage.GetMessage())
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			w.handleEvent(event)
		}
	}
	return firstErr
}

func (w *WhatsAppClient) Disconnect() {
	w.Client.Disconnect()
}

func (w *WhatsAppClient) IsConnected() bool {
	return w.ready.Load() || w.Client.IsConnected()
}

// SendText sends a text message. When reply is non-nil, the message quotes
// that earlier message. sent remains true when only the local cache write fails.
func (w *WhatsAppClient) SendText(ctx context.Context, jid types.JID, text string, reply *ChatMessage) (sent bool, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return false, fmt.Errorf("pesan tidak boleh kosong")
	}
	if !w.Client.IsConnected() {
		if _, err := w.store.EnqueueText(jid, text, reply); err != nil {
			return false, fmt.Errorf("menyimpan outbox: %w", err)
		}
		return false, ErrOutboxQueued
	}
	return deliverText(ctx, w.store, jid, text, reply, func(ctx context.Context, jid types.JID, message *waE2E.Message) (whatsmeow.SendResponse, error) {
		return w.Client.SendMessage(ctx, jid, message)
	})
}

type sendMessageFunc func(context.Context, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error)

func deliverText(ctx context.Context, store *ChatStore, jid types.JID, text string, reply *ChatMessage, send sendMessageFunc) (bool, error) {
	return deliverTextWithID(ctx, store, jid, text, reply, "", send)
}

func deliverTextWithID(ctx context.Context, store *ChatStore, jid types.JID, text string, reply *ChatMessage, messageID types.MessageID, send sendMessageFunc) (bool, error) {
	message := &waE2E.Message{Conversation: proto.String(text)}
	replyID, replyPreview := "", ""
	if reply != nil && reply.ID != "" {
		participant := reply.SenderJID
		if participant == "" {
			participant = jid.String()
		}
		info := &waE2E.ContextInfo{
			StanzaID:    proto.String(reply.ID),
			Participant: proto.String(participant),
		}
		if reply.Text != "" {
			info.QuotedMessage = &waE2E.Message{Conversation: proto.String(reply.Text)}
		}
		message = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(text),
				ContextInfo: info,
			},
		}
		replyID = reply.ID
		replyPreview = reply.Text
	}

	return deliverOutgoing(ctx, store, jid, outgoingMessage{
		message: message, text: text, replyID: replyID, replyPreview: replyPreview, id: messageID,
	}, send)
}

func (w *WhatsAppClient) handleEvent(raw any) {
	switch evt := raw.(type) {
	case *events.Connected:
		w.ready.Store(true)
	case *events.Disconnected:
		w.ready.Store(false)
	case *events.LoggedOut:
		w.ready.Store(false)
	case *events.Receipt:
		ids := make([]string, 0, len(evt.MessageIDs))
		for _, id := range evt.MessageIDs {
			ids = append(ids, string(id))
		}
		switch evt.Type {
		case types.ReceiptTypeDelivered:
			w.logCacheError(w.store.SetDelivery(evt.Chat.String(), ids, DeliveryDelivered))
		case types.ReceiptTypeRead:
			w.logCacheError(w.store.SetDelivery(evt.Chat.String(), ids, DeliveryRead))
		case types.ReceiptTypePlayed:
			w.logCacheError(w.store.SetDelivery(evt.Chat.String(), ids, DeliveryPlayed))
		case types.ReceiptTypeReadSelf:
			w.logCacheError(w.store.MarkIncomingRead(evt.Chat.String(), ids))
		}
	case *events.DeleteForMe:
		w.logCacheError(w.store.DeleteMessage(evt.ChatJID.String(), evt.MessageID))
	case *events.HistorySync:
		if err := w.handleHistorySync(evt.Data, w.Client.ParseWebMessage); err != nil && w.logger != nil {
			w.logger.Errorf("gagal memproses riwayat: %v", err)
		}
	case *events.Message:
		if evt.Message == nil {
			return
		}
		if reaction := evt.Message.GetReactionMessage(); reaction != nil {
			chatJID := reaction.GetKey().GetRemoteJID()
			if chatJID == "" {
				chatJID = evt.Info.Chat.String()
			}
			w.logCacheError(w.store.SetReaction(chatJID, reaction.GetKey().GetID(), evt.Info.Sender.String(), reaction.GetText()))
			return
		}
		if protocol := evt.Message.GetProtocolMessage(); protocol != nil && protocol.GetType() == waE2E.ProtocolMessage_REVOKE {
			chatJID := protocol.GetKey().GetRemoteJID()
			if chatJID == "" {
				chatJID = evt.Info.Chat.String()
			}
			w.logCacheError(w.store.DeleteMessage(chatJID, protocol.GetKey().GetID()))
			return
		}
		_, mediaKind, mediaMIME := mediaContent(evt.Message)
		labelType := evt.Info.MediaType
		if labelType == "" {
			labelType = mediaKind
		}
		text := messageText(evt.Message)
		if label := mediaLabel(labelType); label != "" {
			if text == "" {
				text = label
			} else {
				text = label + " · " + text
			}
		}
		if text == "" {
			return
		}
		if evt.IsEdit {
			w.logCacheError(w.store.EditMessage(evt.Info.Chat.String(), string(evt.Info.ID), text))
			return
		}
		title := evt.Info.PushName
		sender := evt.Info.Sender.String()
		if evt.Info.IsGroup {
			sender = title
			title = "" // PushName adalah nama pengirim, bukan nama grup.
		}
		replyID, replyPreview := replyInfo(evt.Message)
		delivery := DeliveryNone
		if evt.Info.IsFromMe {
			delivery = DeliverySent
		}
		var rawMessage []byte
		if mediaKind != "" {
			rawMessage, _ = proto.Marshal(evt.Message)
		}
		err := w.store.AddMessage(ChatMessage{
			ID:           string(evt.Info.ID),
			ChatJID:      evt.Info.Chat.String(),
			Sender:       sender,
			SenderJID:    evt.Info.Sender.String(),
			Text:         text,
			Timestamp:    evt.Info.Timestamp,
			FromMe:       evt.Info.IsFromMe,
			ReplyToID:    replyID,
			ReplyPreview: replyPreview,
			Unread:       !evt.Info.IsFromMe && evt.SourceWebMsg == nil,
			Delivery:     delivery,
			RawMessage:   rawMessage,
			MediaType:    mediaKind,
			MediaMIME:    mediaMIME,
		}, title)
		w.logCacheError(err)
		if evt.Info.IsGroup {
			w.ensureGroupName(evt.Info.Chat)
		}
	}
}

// ensureGroupName fetches a group's name in the background, once per session.
// It runs after the message is stored so the chat already exists.
func (w *WhatsAppClient) ensureGroupName(jid types.JID) {
	key := jid.String()
	if _, loaded := w.namedGroups.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		info, err := w.Client.GetGroupInfo(ctx, jid)
		if err != nil || info == nil || info.Name == "" {
			// Allow a retry on the next message from this group.
			w.namedGroups.Delete(key)
			return
		}
		w.logCacheError(w.store.SetTitle(key, info.Name))
	}()
}

func (w *WhatsAppClient) logCacheError(err error) {
	if err != nil && w.logger != nil {
		w.logger.Errorf("gagal menyimpan cache chat: %v", err)
	}
}

type markReadFunc func(context.Context, []types.MessageID, time.Time, types.JID, types.JID) error

func (w *WhatsAppClient) MarkChatRead(ctx context.Context, chatJID types.JID) error {
	if !w.Client.IsConnected() {
		return fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}
	return markChatRead(ctx, w.store, chatJID, func(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID) error {
		return w.Client.MarkRead(ctx, ids, timestamp, chat, sender)
	})
}

func markChatRead(ctx context.Context, store *ChatStore, chatJID types.JID, mark markReadFunc) error {
	groups := make(map[string][]types.MessageID)
	for _, chat := range store.Chats() {
		if chat.JID != chatJID.String() {
			continue
		}
		for _, message := range chat.Messages {
			if message.Unread && !message.Deleted {
				groups[message.SenderJID] = append(groups[message.SenderJID], types.MessageID(message.ID))
			}
		}
	}
	if len(groups) == 0 {
		return nil
	}
	senders := make([]string, 0, len(groups))
	for sender := range groups {
		senders = append(senders, sender)
	}
	sort.Strings(senders)
	for _, sender := range senders {
		var senderJID types.JID
		var err error
		if chatJID.Server == types.GroupServer {
			senderJID, err = types.ParseJID(sender)
			if err != nil {
				return fmt.Errorf("alamat pengirim tidak valid: %w", err)
			}
		}
		if err := mark(ctx, groups[sender], time.Now(), chatJID, senderJID); err != nil {
			return err
		}
	}
	return store.MarkChatRead(chatJID.String())
}

func messageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if value := msg.GetConversation(); value != "" {
		return value
	}
	if extended := msg.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	if image := msg.GetImageMessage(); image != nil {
		return image.GetCaption()
	}
	if video := msg.GetVideoMessage(); video != nil {
		return video.GetCaption()
	}
	if document := msg.GetDocumentMessage(); document != nil {
		return document.GetCaption()
	}
	return ""
}

// replyInfo returns the ID of the message this one quotes and a short preview
// of it, or empty strings when the message is not a reply.
func replyInfo(msg *waE2E.Message) (string, string) {
	info := contextInfo(msg)
	if info == nil {
		return "", ""
	}
	preview := ""
	if quoted := info.GetQuotedMessage(); quoted != nil {
		preview = messageText(quoted)
	}
	return info.GetStanzaID(), preview
}

func contextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetContextInfo()
	}
	return nil
}

// mediaLabel turns WhatsApp's media type into a short label for the chat view.
func mediaLabel(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "":
		return ""
	case "image":
		return "📷 Foto"
	case "video", "gif":
		return "🎥 Video"
	case "audio":
		return "🎵 Audio"
	case "ptt":
		return "🎤 Pesan suara"
	case "document":
		return "📄 Dokumen"
	case "sticker":
		return "🏷 Stiker"
	default:
		return "[" + strings.ToLower(mediaType) + "]"
	}
}
