package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func selectionFixture() tuiModel {
	chat := Chat{JID: "chat"}
	for i, id := range []string{"m1", "m2", "m3"} {
		chat.Messages = append(chat.Messages, ChatMessage{
			ID: id, ChatJID: "chat", Sender: "Rani", Text: "text-" + id,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		})
	}
	return tuiModel{chats: []Chat{chat}, selectedJID: "chat", focus: focusComposer, width: 80, height: 24}
}

func pressMessageKey(t *testing.T, model tuiModel, key tea.KeyMsg) (tuiModel, tea.Cmd) {
	t.Helper()
	next, cmd := model.handleKey(key)
	return next.(tuiModel), cmd
}

func TestMessageSelectionReplyAndCopy(t *testing.T) {
	model := selectionFixture()
	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyCtrlG})
	if model.focus != focusMessage || model.selectedMessageID != "m3" {
		t.Fatalf("message selection did not start at newest message: focus=%v id=%q", model.focus, model.selectedMessageID)
	}
	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyUp})
	if model.selectedMessageID != "m2" {
		t.Fatalf("selection moved to %q, want m2", model.selectedMessageID)
	}
	if view := strings.Join(model.renderConversation(60, 20), "\n"); !strings.Contains(view, "▸") {
		t.Fatalf("selected message is not highlighted: %q", view)
	}

	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if model.focus != focusComposer || model.reply == nil || model.reply.ID != "m2" {
		t.Fatalf("reply did not use selected message: focus=%v reply=%#v", model.focus, model.reply)
	}

	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyCtrlG})
	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyUp})
	model, cmd := pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("copy did not return a terminal clipboard command")
	}
	if output := fmt.Sprint(cmd()); !strings.Contains(output, "\x1b]52;c;") || !strings.Contains(output, "dGV4dC1tMg==") {
		t.Fatalf("clipboard command has wrong payload: %q", output)
	}
	if !strings.Contains(strings.ToLower(model.toast), "clipboard") {
		t.Fatalf("copy feedback missing: %q", model.toast)
	}
}

func TestMessageSelectionStaysInsideChat(t *testing.T) {
	model := selectionFixture()
	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyCtrlG})
	for i := 0; i < 10; i++ {
		model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyUp})
	}
	if model.selectedMessageID != "m1" {
		t.Fatalf("selection escaped oldest message: %q", model.selectedMessageID)
	}
	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.focus != focusComposer || model.selectedMessageID != "" {
		t.Fatalf("escape did not leave selection cleanly: focus=%v id=%q", model.focus, model.selectedMessageID)
	}
}

func TestMessageSelectionScrollsIntoView(t *testing.T) {
	chat := Chat{JID: "chat"}
	for i := 0; i < 30; i++ {
		chat.Messages = append(chat.Messages, ChatMessage{ID: fmt.Sprintf("m%02d", i), ChatJID: "chat", Text: fmt.Sprintf("message-%02d", i), Timestamp: time.Now().Add(time.Duration(i) * time.Second)})
	}
	model := tuiModel{chats: []Chat{chat}, selectedJID: "chat", width: 76, height: 18}
	model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyCtrlG})
	for i := 0; i < 20; i++ {
		model, _ = pressMessageKey(t, model, tea.KeyMsg{Type: tea.KeyUp})
	}
	if model.selectedMessageID != "m09" {
		t.Fatalf("selected wrong message: %q", model.selectedMessageID)
	}
	view := strings.Join(model.renderConversation(48, 10), "\n")
	if !strings.Contains(view, "▸") {
		t.Fatalf("selected message moved outside viewport: %q", view)
	}
}
