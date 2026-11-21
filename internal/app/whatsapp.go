package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

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

		fmt.Println("\nWhatsFuwa: scan QR ini lewat WhatsApp > Perangkat tertaut > Tautkan perangkat.\n")
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

func (w *WhatsAppClient) SendText(ctx context.Context, jid types.JID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("pesan tidak boleh kosong")
	}
	if !w.Client.IsConnected() {
		return fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}
	resp, err := w.Client.SendMessage(ctx, jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return err
	}
	return w.store.AddMessage(ChatMessage{
		ID:        string(resp.ID),
		ChatJID:   jid.String(),
		Text:      text,
		Timestamp: resp.Timestamp,
		FromMe:    true,
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
		if text == "" {
			text = nonTextLabel(evt.Info.MediaType)
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
		err := w.store.AddMessage(ChatMessage{
			ID:        string(evt.Info.ID),
			ChatJID:   evt.Info.Chat.String(),
			Sender:    sender,
			Text:      text,
			Timestamp: evt.Info.Timestamp,
			FromMe:    evt.Info.IsFromMe,
		}, title)
		_ = err // The TUI refreshes the bounded local cache periodically.
	}
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

func nonTextLabel(mediaType string) string {
	if mediaType == "" {
		return ""
	}
	return "[" + strings.ToLower(mediaType) + "]"
}
