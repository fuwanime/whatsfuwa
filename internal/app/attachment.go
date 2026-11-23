package app

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type mediaUploader func(context.Context, []byte, whatsmeow.MediaType) (whatsmeow.UploadResponse, error)

type outgoingMessage struct {
	message      *waE2E.Message
	text         string
	mediaType    string
	mediaMIME    string
	mediaPath    string
	replyID      string
	replyPreview string
	id           types.MessageID
}

func (w *WhatsAppClient) SendAttachment(ctx context.Context, jid types.JID, path, caption string, reply *ChatMessage) (bool, error) {
	if !w.Client.IsConnected() {
		return false, fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}
	return sendAttachment(ctx, w.store, jid, path, caption, reply, w.Client.Upload, func(ctx context.Context, jid types.JID, message *waE2E.Message) (whatsmeow.SendResponse, error) {
		return w.Client.SendMessage(ctx, jid, message)
	})
}

func sendAttachment(ctx context.Context, store *ChatStore, jid types.JID, path, caption string, reply *ChatMessage, upload mediaUploader, send sendMessageFunc) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("lampiran harus berupa file biasa")
	}
	if info.Size() > maxMediaBytes {
		return false, fmt.Errorf("lampiran terlalu besar; batas %d MiB", maxMediaBytes>>20)
	}
	if info.Size() == 0 {
		return false, fmt.Errorf("lampiran tidak boleh kosong")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if int64(len(data)) > maxMediaBytes {
		return false, fmt.Errorf("lampiran terlalu besar; batas %d MiB", maxMediaBytes>>20)
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	caption = strings.TrimSpace(caption)
	contextInfo, replyID, replyPreview := attachmentReplyContext(jid, reply)
	var message *waE2E.Message
	kind := "document"
	uploadType := whatsmeow.MediaDocument
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		kind, uploadType = "image", whatsmeow.MediaImage
	case strings.HasPrefix(mimeType, "video/"):
		kind, uploadType = "video", whatsmeow.MediaVideo
	case strings.HasPrefix(mimeType, "audio/"):
		kind, uploadType = "audio", whatsmeow.MediaAudio
		if caption != "" {
			return false, fmt.Errorf("audio tidak mendukung caption")
		}
	}
	if upload == nil {
		return false, fmt.Errorf("pengunggah media tidak tersedia")
	}
	uploaded, err := upload(ctx, data, uploadType)
	if err != nil {
		return false, err
	}
	fileLength := uploaded.FileLength
	if fileLength == 0 {
		fileLength = uint64(len(data))
	}
	switch kind {
	case "image":
		message = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption: optionalString(caption), Mimetype: proto.String(mimeType), URL: proto.String(uploaded.URL),
			DirectPath: proto.String(uploaded.DirectPath), MediaKey: uploaded.MediaKey,
			FileSHA256: uploaded.FileSHA256, FileEncSHA256: uploaded.FileEncSHA256, FileLength: proto.Uint64(fileLength),
			ContextInfo: contextInfo,
		}}
	case "video":
		message = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			Caption: optionalString(caption), Mimetype: proto.String(mimeType), URL: proto.String(uploaded.URL),
			DirectPath: proto.String(uploaded.DirectPath), MediaKey: uploaded.MediaKey,
			FileSHA256: uploaded.FileSHA256, FileEncSHA256: uploaded.FileEncSHA256, FileLength: proto.Uint64(fileLength),
			ContextInfo: contextInfo,
		}}
	case "audio":
		message = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			Mimetype: proto.String(mimeType), URL: proto.String(uploaded.URL), DirectPath: proto.String(uploaded.DirectPath),
			MediaKey: uploaded.MediaKey, FileSHA256: uploaded.FileSHA256, FileEncSHA256: uploaded.FileEncSHA256,
			FileLength: proto.Uint64(fileLength), ContextInfo: contextInfo, PTT: proto.Bool(false),
		}}
	default:
		message = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			Caption: optionalString(caption), Mimetype: proto.String(mimeType), FileName: proto.String(filepath.Base(path)),
			URL: proto.String(uploaded.URL), DirectPath: proto.String(uploaded.DirectPath), MediaKey: uploaded.MediaKey,
			FileSHA256: uploaded.FileSHA256, FileEncSHA256: uploaded.FileEncSHA256, FileLength: proto.Uint64(fileLength),
			ContextInfo: contextInfo,
		}}
	}
	text := mediaLabel(kind)
	if caption != "" {
		text += " · " + caption
	}
	return deliverOutgoing(ctx, store, jid, outgoingMessage{
		message: message, text: text, mediaType: kind, mediaMIME: mimeType, mediaPath: path,
		replyID: replyID, replyPreview: replyPreview,
	}, send)
}

func attachmentReplyContext(jid types.JID, reply *ChatMessage) (*waE2E.ContextInfo, string, string) {
	if reply == nil || reply.ID == "" {
		return nil, "", ""
	}
	participant := reply.SenderJID
	if participant == "" {
		participant = jid.String()
	}
	info := &waE2E.ContextInfo{StanzaID: proto.String(reply.ID), Participant: proto.String(participant)}
	if reply.Text != "" {
		info.QuotedMessage = &waE2E.Message{Conversation: proto.String(reply.Text)}
	}
	return info, reply.ID, reply.Text
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return proto.String(value)
}

func deliverOutgoing(ctx context.Context, store *ChatStore, jid types.JID, outgoing outgoingMessage, send sendMessageFunc) (bool, error) {
	if send == nil {
		return false, fmt.Errorf("pengirim pesan tidak tersedia")
	}
	resp, err := send(ctx, jid, outgoing.message)
	if err != nil {
		return false, err
	}
	if outgoing.id != "" {
		resp.ID = outgoing.id
	}
	var rawMessage []byte
	if outgoing.mediaType != "" {
		rawMessage, _ = proto.Marshal(outgoing.message)
	}
	if err := store.AddMessage(ChatMessage{
		ID: string(resp.ID), ChatJID: jid.String(), Text: outgoing.text, Timestamp: resp.Timestamp,
		FromMe: true, ReplyToID: outgoing.replyID, ReplyPreview: outgoing.replyPreview, Delivery: DeliverySent,
		RawMessage: rawMessage, MediaType: outgoing.mediaType, MediaMIME: outgoing.mediaMIME, MediaPath: outgoing.mediaPath,
	}, ""); err != nil {
		return true, fmt.Errorf("menyimpan cache pesan terkirim: %w", err)
	}
	return true, nil
}
