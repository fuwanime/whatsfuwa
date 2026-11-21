package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type WhatsAppClient struct {
	Client *whatsmeow.Client
	store  *ChatStore
	ready  atomic.Bool
	// namedGroups records group JIDs whose name has already been requested
	// this session, so each group is looked up at most once.
	namedGroups sync.Map
}

func NewWhatsAppClient(device *store.Device, logger waLog.Logger, chatStore *ChatStore) *WhatsAppClient {
	wa := &WhatsAppClient{
		Client: whatsmeow.NewClient(device, logger),
		store:  chatStore,
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
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			case "success":
				fmt.Println("\nLogin berhasil. WhatsFuwa tersambung.")
			case "timeout":
				fmt.Println("QR kedaluwarsa. WhatsFuwa akan mencoba lagi saat koneksi tersedia.")
			case "error":
				if evt.Error != nil {
					return fmt.Errorf("login WhatsApp gagal: %w", evt.Error)
				}
			case "err-client-outdated":
				return fmt.Errorf("versi protokol WhatsApp perlu diperbarui; update WhatsFuwa")
			case "err-scanned-without-multidevice":
				return fmt.Errorf("akun belum mengaktifkan fitur multi-perangkat")
			default:
				fmt.Printf("Status pairing: %s\n", evt.Event)
			}
		}
		return nil
	}

	if err := w.Client.ConnectContext(ctx); err != nil {
		return fmt.Errorf("menghubungkan sesi tersimpan: %w", err)
	}
	fmt.Println("Sesi ditemukan. Menghubungkan kembali...")
	return nil
}

func (w *WhatsAppClient) Disconnect() {
	w.Client.Disconnect()
}

func (w *WhatsAppClient) IsConnected() bool {
	return w.ready.Load() || w.Client.IsConnected()
}

// SendText sends a text message. When reply is non-nil, the message quotes
// that earlier message.
func (w *WhatsAppClient) SendText(ctx context.Context, jid types.JID, text string, reply *ChatMessage) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("pesan tidak boleh kosong")
	}
	if !w.Client.IsConnected() {
		return fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}

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

	resp, err := w.Client.SendMessage(ctx, jid, message)
	if err != nil {
		return err
	}
	return w.store.AddMessage(ChatMessage{
		ID:           string(resp.ID),
		ChatJID:      jid.String(),
		Text:         text,
		Timestamp:    resp.Timestamp,
		FromMe:       true,
		ReplyToID:    replyID,
		ReplyPreview: replyPreview,
	}, "")
}

func (w *WhatsAppClient) handleEvent(raw any) {
	switch evt := raw.(type) {
	case *events.Connected:
		w.ready.Store(true)
	case *events.Disconnected:
		w.ready.Store(false)
	case *events.LoggedOut:
		w.ready.Store(false)
	case *events.Message:
		if evt.Message == nil {
			return
		}
		text := messageText(evt.Message)
		if label := mediaLabel(evt.Info.MediaType); label != "" {
			if text == "" {
				text = label
			} else {
				text = label + " · " + text
			}
		}
		if text == "" {
			return
		}
		title := evt.Info.PushName
		sender := evt.Info.Sender.String()
		if evt.Info.IsGroup {
			sender = title
			title = "" // PushName adalah nama pengirim, bukan nama grup.
		}
		replyID, replyPreview := replyInfo(evt.Message)
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
		}, title)
		_ = err // The TUI refreshes the bounded local cache periodically.
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
		_ = w.store.SetTitle(key, info.Name)
	}()
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
