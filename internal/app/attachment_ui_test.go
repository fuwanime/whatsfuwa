package app

import (
	"errors"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func attachmentModel(t *testing.T) tuiModel {
	store, err := OpenChatStore(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	jid := "628123456789@s.whatsapp.net"
	store.AddChat(jid, "")
	return tuiModel{ui: &UI{store: store}, chats: store.Chats(), selectedJID: jid, focus: focusComposer}
}

func TestAttachmentModeAcceptsFilePath(t *testing.T) {
	model := attachmentModel(t)
	next, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	model = next.(tuiModel)
	if !model.attaching || model.focus != focusAttachment {
		t.Fatalf("attachment mode did not open: attaching=%v focus=%v", model.attaching, model.focus)
	}
	next, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/tmp/a.png")})
	model = next.(tuiModel)
	if model.attachPath != "/tmp/a.png" {
		t.Fatalf("path routed to wrong input: %q", model.attachPath)
	}
}

func TestAttachmentFailureRestoresPath(t *testing.T) {
	model := attachmentModel(t)
	next, _ := model.Update(attachmentResultMsg{
		jid: model.selectedJID, path: "/tmp/a.png", err: errors.New("upload failed"),
	})
	model = next.(tuiModel)
	if !model.attaching || model.focus != focusAttachment || model.attachPath != "/tmp/a.png" {
		t.Fatalf("failed path was not restored: attaching=%v focus=%v path=%q", model.attaching, model.focus, model.attachPath)
	}
}

func TestAcceptedAttachmentDoesNotRestoreForm(t *testing.T) {
	model := attachmentModel(t)
	next, _ := model.Update(attachmentResultMsg{
		jid: model.selectedJID, path: "/tmp/a.png", sent: true, err: errors.New("cache failed"),
	})
	model = next.(tuiModel)
	if model.attaching || model.attachPath != "" {
		t.Fatalf("accepted attachment restored form: %#v", model)
	}
	if model.toast == "" || !model.toastIsErr {
		t.Fatalf("cache warning was not preserved: %q", model.toast)
	}
}
