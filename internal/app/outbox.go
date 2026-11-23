package app

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var ErrOutboxQueued = errors.New("pesan disimpan di outbox")

type outboxSendFactory func(types.MessageID) sendMessageFunc

func (w *WhatsAppClient) RetryOutbox(ctx context.Context) (int, error) {
	if !w.Client.IsConnected() {
		return 0, fmt.Errorf("WhatsFuwa belum terhubung ke WhatsApp")
	}
	return retryOutbox(ctx, w.store, func(id types.MessageID) sendMessageFunc {
		return func(ctx context.Context, jid types.JID, message *waE2E.Message) (whatsmeow.SendResponse, error) {
			return w.Client.SendMessage(ctx, jid, message, whatsmeow.SendRequestExtra{ID: id})
		}
	})
}

func retryOutbox(ctx context.Context, store *ChatStore, factory outboxSendFactory) (int, error) {
	sentCount := 0
	for _, entry := range store.Outbox() {
		jid, err := types.ParseJID(entry.ChatJID)
		if err != nil {
			return sentCount, err
		}
		sent, sendErr := deliverTextWithID(ctx, store, jid, entry.Text, entry.Reply, types.MessageID(entry.ID), factory(types.MessageID(entry.ID)))
		if !sent {
			if err := store.RecordOutboxAttempt(entry.ID, sendErr.Error()); err != nil {
				return sentCount, fmt.Errorf("mencatat kegagalan outbox: %w", err)
			}
			return sentCount, sendErr
		}
		if err := store.RemoveOutbox(entry.ID); err != nil {
			return sentCount, fmt.Errorf("pesan diterima server, tetapi outbox gagal dihapus: %w", err)
		}
		sentCount++
		if sendErr != nil {
			return sentCount, sendErr
		}
	}
	return sentCount, nil
}
