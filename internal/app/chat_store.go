package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	maxChats       = 80
	maxMessages    = 500
	maxMessageText = 1200
)

type DeliveryStatus uint8

const (
	DeliveryNone DeliveryStatus = iota
	DeliverySent
	DeliveryDelivered
	DeliveryRead
	DeliveryPlayed
)

type ChatMessage struct {
	ID        string    `json:"id"`
	ChatJID   string    `json:"chat_jid"`
	Sender    string    `json:"sender,omitempty"`
	SenderJID string    `json:"sender_jid,omitempty"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	FromMe    bool      `json:"from_me"`
	// ReplyToID is the ID of the message this one replies to, and
	// ReplyPreview is a short copy of that message's text for display.
	ReplyToID    string            `json:"reply_to_id,omitempty"`
	ReplyPreview string            `json:"reply_preview,omitempty"`
	Edited       bool              `json:"edited,omitempty"`
	Deleted      bool              `json:"deleted,omitempty"`
	Reactions    map[string]string `json:"reactions,omitempty"`
	Unread       bool              `json:"unread,omitempty"`
	Delivery     DeliveryStatus    `json:"delivery,omitempty"`
	RawMessage   []byte            `json:"raw_message,omitempty"`
	MediaType    string            `json:"media_type,omitempty"`
	MediaMIME    string            `json:"media_mime,omitempty"`
	MediaPath    string            `json:"media_path,omitempty"`
}

type Chat struct {
	JID         string        `json:"jid"`
	Title       string        `json:"title,omitempty"`
	LastMessage string        `json:"last_message,omitempty"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Unread      int           `json:"unread,omitempty"`
	Messages    []ChatMessage `json:"messages"`
}

type OutboxMessage struct {
	ID        string       `json:"id"`
	ChatJID   string       `json:"chat_jid"`
	Text      string       `json:"text"`
	Reply     *ChatMessage `json:"reply,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Attempts  int          `json:"attempts,omitempty"`
	LastError string       `json:"last_error,omitempty"`
}

type chatFile struct {
	Version int             `json:"version"`
	Chats   []Chat          `json:"chats"`
	Outbox  []OutboxMessage `json:"outbox,omitempty"`
}

type ChatStore struct {
	mu     sync.RWMutex
	path   string
	chats  map[string]*Chat
	outbox []OutboxMessage
}

func OpenChatStore(path string) (*ChatStore, error) {
	store := &ChatStore{path: path, chats: make(map[string]*Chat)}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}

	var saved chatFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	store.outbox = append([]OutboxMessage(nil), saved.Outbox...)
	for _, chat := range saved.Chats {
		copyChat := chat
		normalizeChatMessages(&copyChat)
		store.chats[copyChat.JID] = &copyChat
	}
	store.trimLocked()
	return store, nil
}

func (s *ChatStore) EnqueueText(jid types.JID, text string, reply *ChatMessage) (OutboxMessage, error) {
	text = strings.TrimSpace(text)
	if jid.IsEmpty() || text == "" {
		return OutboxMessage{}, fmt.Errorf("pesan outbox tidak valid")
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return OutboxMessage{}, err
	}
	entry := OutboxMessage{
		ID: strings.ToUpper(hex.EncodeToString(idBytes)), ChatJID: jid.String(), Text: text,
		CreatedAt: time.Now(),
	}
	if reply != nil {
		copyReply := *reply
		entry.Reply = &copyReply
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbox = append(s.outbox, entry)
	if err := s.saveLocked(); err != nil {
		s.outbox = s.outbox[:len(s.outbox)-1]
		return OutboxMessage{}, err
	}
	return entry, nil
}

func (s *ChatStore) Outbox() []OutboxMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]OutboxMessage, len(s.outbox))
	for i, entry := range s.outbox {
		result[i] = entry
		if entry.Reply != nil {
			copyReply := *entry.Reply
			result[i].Reply = &copyReply
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (s *ChatStore) RemoveOutbox(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]OutboxMessage(nil), s.outbox...)
	for i, entry := range s.outbox {
		if entry.ID == id {
			s.outbox = append(s.outbox[:i], s.outbox[i+1:]...)
			if err := s.saveLocked(); err != nil {
				s.outbox = previous
				return err
			}
			return nil
		}
	}
	return nil
}

func (s *ChatStore) RecordOutboxAttempt(id, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]OutboxMessage(nil), s.outbox...)
	for i := range s.outbox {
		if s.outbox[i].ID == id {
			s.outbox[i].Attempts++
			s.outbox[i].LastError = lastError
			if err := s.saveLocked(); err != nil {
				s.outbox = previous
				return err
			}
			return nil
		}
	}
	return nil
}

func (s *ChatStore) AddMessage(message ChatMessage, title string) error {
	if message.ChatJID == "" || message.ID == "" {
		return nil
	}
	message.Text = strings.TrimSpace(message.Text)
	if runes := []rune(message.Text); len(runes) > maxMessageText {
		message.Text = string(runes[:maxMessageText]) + "…"
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[message.ChatJID]
	if chat == nil {
		chat = &Chat{JID: message.ChatJID}
		s.chats[message.ChatJID] = chat
	}
	if title != "" && (chat.Title == "" || !strings.Contains(message.ChatJID, "@g.us")) {
		chat.Title = title
	}
	for _, old := range chat.Messages {
		if old.ID == message.ID {
			return nil
		}
	}
	chat.Messages = append(chat.Messages, message)
	normalizeChatMessages(chat)
	s.trimLocked()
	return s.saveLocked()
}

func normalizeChatMessages(chat *Chat) {
	sort.SliceStable(chat.Messages, func(i, j int) bool {
		return chat.Messages[i].Timestamp.Before(chat.Messages[j].Timestamp)
	})
	if len(chat.Messages) > maxMessages {
		chat.Messages = chat.Messages[len(chat.Messages)-maxMessages:]
	}
	chat.Unread = 0
	if len(chat.Messages) == 0 {
		return
	}
	for _, message := range chat.Messages {
		if message.Unread && !message.Deleted {
			chat.Unread++
		}
	}
	newest := chat.Messages[len(chat.Messages)-1]
	chat.LastMessage = newest.Text
	if newest.Deleted {
		chat.LastMessage = "Pesan dihapus"
	}
	chat.UpdatedAt = newest.Timestamp
}

func (s *ChatStore) EditMessage(chatJID, messageID, text string) error {
	return s.updateMessage(chatJID, messageID, func(message *ChatMessage) {
		message.Text = strings.TrimSpace(text)
		message.Edited = true
		message.Deleted = false
	})
}

func (s *ChatStore) DeleteMessage(chatJID, messageID string) error {
	return s.updateMessage(chatJID, messageID, func(message *ChatMessage) {
		message.Deleted = true
	})
}

func (s *ChatStore) SetReaction(chatJID, messageID, sender, emoji string) error {
	return s.updateMessage(chatJID, messageID, func(message *ChatMessage) {
		if message.Reactions == nil {
			message.Reactions = make(map[string]string)
		}
		if emoji == "" {
			delete(message.Reactions, sender)
		} else {
			message.Reactions[sender] = emoji
		}
	})
}

func (s *ChatStore) updateMessage(chatJID, messageID string, update func(*ChatMessage)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[chatJID]
	if chat == nil {
		return nil
	}
	for i := range chat.Messages {
		if chat.Messages[i].ID == messageID {
			update(&chat.Messages[i])
			normalizeChatMessages(chat)
			return s.saveLocked()
		}
	}
	return nil
}

func (s *ChatStore) SetMediaPath(chatJID, messageID, path, mimeType string) error {
	return s.updateMessage(chatJID, messageID, func(message *ChatMessage) {
		message.MediaPath = path
		if mimeType != "" {
			message.MediaMIME = mimeType
		}
	})
}

func (s *ChatStore) SetDelivery(chatJID string, messageIDs []string, status DeliveryStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[chatJID]
	if chat == nil {
		return nil
	}
	changed := false
	for _, id := range messageIDs {
		for i := range chat.Messages {
			if chat.Messages[i].ID == id && status > chat.Messages[i].Delivery {
				chat.Messages[i].Delivery = status
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *ChatStore) MarkIncomingRead(chatJID string, messageIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[chatJID]
	if chat == nil {
		return nil
	}
	for _, id := range messageIDs {
		for i := range chat.Messages {
			if chat.Messages[i].ID == id {
				chat.Messages[i].Unread = false
			}
		}
	}
	normalizeChatMessages(chat)
	return s.saveLocked()
}

func (s *ChatStore) MarkChatRead(chatJID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[chatJID]
	if chat == nil {
		return nil
	}
	for i := range chat.Messages {
		chat.Messages[i].Unread = false
	}
	normalizeChatMessages(chat)
	return s.saveLocked()
}

func (s *ChatStore) AddChat(jid, title string) error {
	if jid == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[jid]
	if chat == nil {
		chat = &Chat{JID: jid, UpdatedAt: time.Now()}
		s.chats[jid] = chat
	}
	if title != "" {
		chat.Title = title
	}
	s.trimLocked()
	return s.saveLocked()
}

// SetTitle renames an existing chat. It does nothing if the chat is not cached,
// so it never creates a chat with a made-up timestamp.
func (s *ChatStore) SetTitle(jid, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.chats[jid]
	if chat == nil || title == "" || chat.Title == title {
		return nil
	}
	chat.Title = title
	return s.saveLocked()
}

func (s *ChatStore) Chats() []Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Chat, 0, len(s.chats))
	for _, chat := range s.chats {
		copyChat := *chat
		copyChat.Messages = make([]ChatMessage, len(chat.Messages))
		for i, message := range chat.Messages {
			copyChat.Messages[i] = message
			if message.Reactions != nil {
				copyChat.Messages[i].Reactions = make(map[string]string, len(message.Reactions))
				for sender, emoji := range message.Reactions {
					copyChat.Messages[i].Reactions[sender] = emoji
				}
			}
		}
		result = append(result, copyChat)
	}
	sort.Slice(result, func(i, j int) bool {
		return chatPrecedes(result[i], result[j])
	})
	return result
}

func chatPrecedes(a, b Chat) bool {
	if a.UpdatedAt.Equal(b.UpdatedAt) {
		return a.JID < b.JID
	}
	return a.UpdatedAt.After(b.UpdatedAt)
}

func (s *ChatStore) Find(query string) []Chat {
	query = strings.ToLower(strings.TrimSpace(query))
	chats := s.Chats()
	if query == "" {
		return chats
	}
	filtered := make([]Chat, 0)
	for _, chat := range chats {
		if strings.Contains(strings.ToLower(chat.Title), query) || strings.Contains(strings.ToLower(chat.JID), query) {
			filtered = append(filtered, chat)
		}
	}
	return filtered
}

func (s *ChatStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *ChatStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file := chatFile{Version: 2, Chats: make([]Chat, 0, len(s.chats)), Outbox: append([]OutboxMessage(nil), s.outbox...)}
	sort.Slice(file.Outbox, func(i, j int) bool {
		if file.Outbox[i].CreatedAt.Equal(file.Outbox[j].CreatedAt) {
			return file.Outbox[i].ID < file.Outbox[j].ID
		}
		return file.Outbox[i].CreatedAt.Before(file.Outbox[j].CreatedAt)
	})
	for _, chat := range s.chats {
		file.Chats = append(file.Chats, *chat)
	}
	sort.Slice(file.Chats, func(i, j int) bool {
		return chatPrecedes(file.Chats[i], file.Chats[j])
	})
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *ChatStore) trimLocked() {
	if len(s.chats) <= maxChats {
		return
	}
	ordered := make([]*Chat, 0, len(s.chats))
	for _, chat := range s.chats {
		ordered = append(ordered, chat)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return chatPrecedes(*ordered[i], *ordered[j])
	})
	for _, chat := range ordered[maxChats:] {
		delete(s.chats, chat.JID)
	}
}
