package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxChats       = 80
	maxMessages    = 30
	maxMessageText = 1200
)

type ChatMessage struct {
	ID        string    `json:"id"`
	ChatJID   string    `json:"chat_jid"`
	Sender    string    `json:"sender,omitempty"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	FromMe    bool      `json:"from_me"`
}

type Chat struct {
	JID         string        `json:"jid"`
	Title       string        `json:"title,omitempty"`
	LastMessage string        `json:"last_message,omitempty"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Messages    []ChatMessage `json:"messages"`
}

type chatFile struct {
	Version int    `json:"version"`
	Chats   []Chat `json:"chats"`
}

type ChatStore struct {
	mu    sync.RWMutex
	path  string
	chats map[string]*Chat
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
	for _, chat := range saved.Chats {
		copyChat := chat
		if len(copyChat.Messages) > maxMessages {
			copyChat.Messages = copyChat.Messages[len(copyChat.Messages)-maxMessages:]
		}
		store.chats[copyChat.JID] = &copyChat
	}
	store.trimLocked()
	return store, nil
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
	if len(chat.Messages) > maxMessages {
		chat.Messages = chat.Messages[len(chat.Messages)-maxMessages:]
	}
	chat.LastMessage = message.Text
	chat.UpdatedAt = message.Timestamp
	s.trimLocked()
	return s.saveLocked()
}

func (s *ChatStore) AddChat(jid, title string) {
	if jid == "" {
		return
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
	_ = s.saveLocked()
}

func (s *ChatStore) Chats() []Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Chat, 0, len(s.chats))
	for _, chat := range s.chats {
		copyChat := *chat
		copyChat.Messages = append([]ChatMessage(nil), chat.Messages...)
		result = append(result, copyChat)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result
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
	file := chatFile{Version: 1, Chats: make([]Chat, 0, len(s.chats))}
	for _, chat := range s.chats {
		file.Chats = append(file.Chats, *chat)
	}
	sort.Slice(file.Chats, func(i, j int) bool {
		return file.Chats[i].UpdatedAt.After(file.Chats[j].UpdatedAt)
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
		return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt)
	})
	for _, chat := range ordered[maxChats:] {
		delete(s.chats, chat.JID)
	}
}
