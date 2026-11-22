package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type recordingLogger struct {
	errors []string
}

func (l *recordingLogger) Warnf(string, ...any)  {}
func (l *recordingLogger) Infof(string, ...any)  {}
func (l *recordingLogger) Debugf(string, ...any) {}
func (l *recordingLogger) Errorf(msg string, args ...any) {
	l.errors = append(l.errors, fmt.Sprintf(msg, args...))
}
func (l *recordingLogger) Sub(string) waLog.Logger { return l }

func failingChatStore(t *testing.T) *ChatStore {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &ChatStore{path: filepath.Join(blocker, "chats.json"), chats: make(map[string]*Chat)}
}

func TestCreateChatReportsCacheFailure(t *testing.T) {
	model := tuiModel{ui: &UI{store: failingChatStore(t)}, newChat: "628123456789"}
	model.createChat()
	if !model.toastIsErr || !strings.Contains(strings.ToLower(model.toast), "cache") {
		t.Fatalf("cache failure was hidden: %q", model.toast)
	}
	if model.selectedJID == "" || model.focus != focusComposer {
		t.Fatalf("in-memory chat was not kept usable: %#v", model)
	}
}

func TestIncomingMessageCacheFailureIsLogged(t *testing.T) {
	logger := &recordingLogger{}
	client := &WhatsAppClient{store: failingChatStore(t), logger: logger}
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	client.handleEvent(&events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: jid, Sender: jid},
			ID:            "message", Timestamp: time.Now(),
		},
		Message: &waE2E.Message{Conversation: proto.String("hello")},
	})
	if len(logger.errors) == 0 || !strings.Contains(strings.ToLower(logger.errors[0]), "cache") {
		t.Fatalf("cache failure was not logged: %#v", logger.errors)
	}
}
