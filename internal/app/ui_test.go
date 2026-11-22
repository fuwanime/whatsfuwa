package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

func TestWrapTextKeepsEveryLineWithinWidth(t *testing.T) {
	lines := wrapText("Ini pesan yang cukup panjang untuk dipotong menjadi beberapa baris tanpa merusak karakter 日本語.", 20)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", lines)
	}
	for _, line := range lines {
		if runewidth.StringWidth(line) > 20 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestWrapTextPreservesLongWordsAndGraphemes(t *testing.T) {
	for _, original := range []string{
		"https://example.com/" + strings.Repeat("a", 50),
		strings.Repeat("👩\u200d💻", 6),
		"e\u0301e\u0301e\u0301e\u0301e\u0301",
	} {
		lines := wrapText(original, 4)
		if got := strings.Join(lines, ""); got != original {
			t.Fatalf("wrapping changed %q into %#v", original, lines)
		}
		for _, line := range lines {
			if got := runewidth.StringWidth(line); got > 4 {
				t.Fatalf("wrapped line %q occupies %d cells", line, got)
			}
		}
	}
}

func TestTruncateUsesTerminalCells(t *testing.T) {
	got := truncate("FuwaStream 日本語", 9)
	if runewidth.StringWidth(got) > 9 || !strings.HasSuffix(got, "…") {
		t.Fatalf("unexpected truncation: %q", got)
	}
}

func TestFitANSIUsesTerminalCells(t *testing.T) {
	for _, test := range []struct {
		value string
		width int
	}{
		{"界界", 4},
		{"e\u0301", 1},
		{"👩\u200d💻", 2},
	} {
		got := fitANSI(test.value, test.width)
		if got != test.value || runewidth.StringWidth(got) != test.width {
			t.Fatalf("fitANSI(%q, %d) = %q (%d cells)", test.value, test.width, got, runewidth.StringWidth(got))
		}
	}
}

func TestUntrustedTextCannotEmitTerminalControls(t *testing.T) {
	model := tuiModel{
		chats: []Chat{{
			JID: "chat", Title: "group\x1b[2J",
			Messages: []ChatMessage{{
				ID: "message", ChatJID: "chat", Sender: "sender\x1b[2J",
				Text: "hello\x1b]52;c;Zm9v\a world\x1b[2J", Timestamp: time.Now(),
			}},
		}},
		selectedJID: "chat",
	}
	view := strings.Join(model.renderConversation(60, 10), "\n") + model.renderSubheader(100) + model.renderChatList(30, 4)[0]
	for _, sequence := range []string{"\x1b]", "\a", "\x1b[2J"} {
		if strings.Contains(view, sequence) {
			t.Fatalf("untrusted terminal control %q reached output: %q", sequence, view)
		}
	}
	if !strings.Contains(view, "hello") || !strings.Contains(view, "world") {
		t.Fatalf("printable message text was lost: %q", view)
	}
}

func TestConversationScrollBoundaries(t *testing.T) {
	chat := Chat{JID: "chat"}
	for i := 0; i < 5; i++ {
		chat.Messages = append(chat.Messages, ChatMessage{ID: string(rune('a' + i)), ChatJID: "chat", Text: "message body", Timestamp: time.Now()})
	}
	t.Run("history indicator survives a full page", func(t *testing.T) {
		model := tuiModel{chats: []Chat{chat}, selectedJID: "chat", scrollOffset: 1}
		view := strings.Join(model.renderConversation(60, 4), "\n")
		if !strings.Contains(view, "Riwayat pesan lebih lama") || !strings.Contains(view, "message body") {
			t.Fatalf("full history page is missing content: %q", view)
		}
	})
	t.Run("overscroll clamps to content", func(t *testing.T) {
		model := tuiModel{chats: []Chat{chat}, selectedJID: "chat", scrollOffset: 1000}
		view := strings.Join(model.renderConversation(60, 4), "\n")
		if !strings.Contains(view, "message body") {
			t.Fatalf("overscroll hid every message: %q", view)
		}
	})
	t.Run("exact fit does not scroll", func(t *testing.T) {
		oneMessage := Chat{JID: "chat", Messages: []ChatMessage{{ID: "one", ChatJID: "chat", Text: "message body", Timestamp: time.Now()}}}
		model := tuiModel{chats: []Chat{oneMessage}, selectedJID: "chat", scrollOffset: 1000, width: 76, height: 18}
		view := strings.Join(model.renderConversation(60, 3), "\n")
		if strings.Contains(view, "Riwayat pesan lebih lama") || !strings.Contains(view, "message body") {
			t.Fatalf("exact-fit content scrolled unexpectedly: %q", view)
		}
		model.scrollOffset = 0
		model.height = 13
		next, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
		model = next.(tuiModel)
		if model.scrollOffset != 0 {
			t.Fatalf("exact-fit page up offset = %d, want 0", model.scrollOffset)
		}
	})
	t.Run("page keys clamp at oldest content", func(t *testing.T) {
		model := tuiModel{chats: []Chat{chat}, selectedJID: "chat", width: 76, height: 18}
		for i := 0; i < 20; i++ {
			next, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
			model = next.(tuiModel)
		}
		if model.scrollOffset != 6 {
			t.Fatalf("page up offset = %d, want oldest-content offset 6", model.scrollOffset)
		}
	})
}

func TestChatListWheelKeepsAFullPage(t *testing.T) {
	model := tuiModel{width: 100, height: 20}
	for i := 0; i < 10; i++ {
		model.chats = append(model.chats, Chat{JID: string(rune('a' + i))})
	}
	for i := 0; i < 10; i++ {
		next, _ := model.handleMouse(tea.MouseMsg{X: 1, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		model = next.(tuiModel)
	}
	if maxOffset := 4; model.listOffset != maxOffset {
		t.Fatalf("chat list offset = %d, want full-page maximum %d", model.listOffset, maxOffset)
	}
}

func TestChatClickUsesTheSameClampedOffsetAsRendering(t *testing.T) {
	store, err := OpenChatStore(t.TempDir() + "/chats.json")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		store.AddChat(string(rune('a'+i)), "")
	}
	model := tuiModel{ui: &UI{store: store}, chats: store.Chats(), width: 100, height: 20, listOffset: 9}
	next, _ := model.handleMouse(tea.MouseMsg{X: 1, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = next.(tuiModel)
	if want := model.chats[4].JID; model.selectedJID != want {
		t.Fatalf("click selected %q, want rendered row %q", model.selectedJID, want)
	}
}

func TestModeTransitionsRouteInputToActiveForm(t *testing.T) {
	newModel := func(t *testing.T) tuiModel {
		store, err := OpenChatStore(t.TempDir() + "/chats.json")
		if err != nil {
			t.Fatal(err)
		}
		store.AddChat("chat", "")
		return tuiModel{
			ui: &UI{store: store}, chats: store.Chats(), selectedJID: "chat",
			width: 100, height: 28, focus: focusComposer,
		}
	}
	typeRune := func(model tuiModel, value string) tuiModel {
		next, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
		return next.(tuiModel)
	}

	t.Run("Search closes Create", func(t *testing.T) {
		model := newModel(t)
		model.creating = true
		model.focus = focusNewChat
		actions := "[ + New ] [ / Search ] [ × Exit ]"
		x := model.width - visibleLen(actions) + len("[ + New ] ") + 1
		next, _ := model.handleMouse(tea.MouseMsg{X: x, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		model = next.(tuiModel)
		if model.creating || !model.searchOpen || model.focus != focusSearch {
			t.Fatalf("incompatible modes: creating=%v search=%v focus=%v", model.creating, model.searchOpen, model.focus)
		}
		next, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		model = typeRune(next.(tuiModel), "x")
		if model.newChat != "" || model.compose != "x" {
			t.Fatalf("rune routed to stale form: compose=%q newChat=%q filter=%q", model.compose, model.newChat, model.filter)
		}
	})

	t.Run("New closes Search", func(t *testing.T) {
		model := newModel(t)
		model.searchOpen = true
		model.focus = focusSearch
		next, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
		model = next.(tuiModel)
		if model.searchOpen || !model.creating || model.focus != focusNewChat {
			t.Fatalf("incompatible modes: creating=%v search=%v focus=%v", model.creating, model.searchOpen, model.focus)
		}
		model = typeRune(model, "6")
		if model.newChat != "6" || model.filter != "" {
			t.Fatalf("rune routed to stale form: newChat=%q filter=%q", model.newChat, model.filter)
		}
	})

	t.Run("chat click closes Search", func(t *testing.T) {
		model := newModel(t)
		model.searchOpen = true
		model.focus = focusSearch
		next, _ := model.handleMouse(tea.MouseMsg{X: 1, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		model = next.(tuiModel)
		model = typeRune(model, "x")
		if model.searchOpen || model.creating || model.filter != "" || model.compose != "x" {
			t.Fatalf("rune routed to stale form: search=%v creating=%v compose=%q filter=%q", model.searchOpen, model.creating, model.compose, model.filter)
		}
	})
}

func TestStatusCannotEmitTerminalControls(t *testing.T) {
	model := tuiModel{toast: "error\x1b]52;c;Zm9v\a", toastIsErr: true}
	if view := model.renderStatus(100); strings.Contains(view, "\x1b]") || strings.ContainsRune(view, '\a') {
		t.Fatalf("terminal control leaked through status: %q", view)
	}
}

func TestDigitsOnly(t *testing.T) {
	if got := digitsOnly("+62 812-3456-7890"); got != "6281234567890" {
		t.Fatalf("unexpected phone normalization: %q", got)
	}
}

func TestTrimLastRune(t *testing.T) {
	if got := trimLastRune("halo界"); got != "halo" {
		t.Fatalf("unexpected backspace result: %q", got)
	}
}
