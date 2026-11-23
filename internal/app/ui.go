package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/whatsmeow/types"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiSelect = "\033[48;5;236m\033[38;5;81m"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type focusTarget uint8

const (
	focusComposer focusTarget = iota
	focusList
	focusSearch
	focusNewChat
	focusMessage
	focusAttachment
)

type UI struct {
	wa    *WhatsAppClient
	store *ChatStore
}

type tuiModel struct {
	ui *UI

	ctx    context.Context
	width  int
	height int

	chats             []Chat
	selectedJID       string
	selectedMessageID string
	selectedIdx       int
	listOffset        int
	scrollOffset      int

	compose    string
	filter     string
	newChat    string
	attachPath string
	attaching  bool
	searchOpen bool
	creating   bool
	focus      focusTarget
	// reply is the message the next sent message will quote, or nil.
	reply           *ChatMessage
	pendingDrafts   map[string]sendResultMsg
	readInFlight    bool
	lastReadAttempt time.Time
	historyPending  map[string]bool

	toast      string
	toastIsErr bool
}

type refreshMsg time.Time

type readResultMsg struct {
	jid string
	err error
}

type historyResultMsg struct {
	jid string
	err error
}

type mediaAction uint8

const (
	mediaDownloadOnly mediaAction = iota
	mediaPreview
	mediaOpenExternal
)

type mediaResultMsg struct {
	message ChatMessage
	action  mediaAction
	path    string
	err     error
}

type mediaOpenResultMsg struct {
	path string
	err  error
}

type attachmentResultMsg struct {
	jid     string
	path    string
	caption string
	sent    bool
	err     error
}

type outboxResultMsg struct {
	sent int
	err  error
}

type sendResultMsg struct {
	jid   string
	text  string
	reply *ChatMessage
	sent  bool
	err   error
}

func NewUI(wa *WhatsAppClient, store *ChatStore) *UI {
	return &UI{wa: wa, store: store}
}

// Run starts WhatsFuwa's full-screen terminal interface. Mouse reporting is
// enabled for terminals that support SGR mouse events; keyboard shortcuts are
// available as a fallback.
func (ui *UI) Run(ctx context.Context) error {
	model := tuiModel{
		ui:     ui,
		ctx:    ctx,
		width:  100,
		height: 28,
		focus:  focusComposer,
	}
	model.refreshChats()
	if len(model.chats) > 0 {
		model.selectedJID = model.chats[0].JID
	} else {
		model.focus = focusList
	}

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			program.Quit()
		case <-done:
		}
	}()
	_, err := program.Run()
	close(done)
	if err != nil {
		return fmt.Errorf("menjalankan antarmuka WhatsFuwa: %w", err)
	}
	return nil
}

func (m tuiModel) Init() tea.Cmd {
	return scheduleRefresh()
}

func (m tuiModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.keepListSelectionVisible()
		return m, nil

	case refreshMsg:
		m.refreshChats()
		return m, tea.Batch(scheduleRefresh(), m.markReadCmd())

	case readResultMsg:
		m.readInFlight = false
		if msg.err != nil {
			m.setToast("Gagal menandai pesan dibaca: "+msg.err.Error(), true)
		} else {
			m.refreshChats()
		}
		return m, nil

	case historyResultMsg:
		delete(m.historyPending, msg.jid)
		if msg.err != nil {
			m.setToast("Gagal meminta riwayat: "+msg.err.Error(), true)
		} else {
			m.setToast("Riwayat diminta. Pesan lama akan muncul setelah diterima.", false)
		}
		return m, nil

	case mediaResultMsg:
		m.refreshChats()
		if msg.path == "" {
			m.setToast("Gagal mengunduh media: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.err != nil {
			m.setToast(msg.err.Error(), true)
		}
		switch msg.action {
		case mediaPreview:
			if msg.message.MediaType == "image" || msg.message.MediaType == "sticker" {
				return m.previewMedia(msg.path)
			}
			return m.openMedia(msg.path)
		case mediaOpenExternal:
			return m.openMedia(msg.path)
		default:
			m.setToast("Media tersimpan di "+msg.path, false)
			return m, nil
		}

	case mediaOpenResultMsg:
		if msg.err != nil {
			m.setToast("Gagal membuka media: "+msg.err.Error()+". File: "+msg.path, true)
		} else {
			m.setToast("Media dibuka: "+msg.path, false)
		}
		return m, nil

	case attachmentResultMsg:
		if !msg.sent {
			if m.selectedJID == msg.jid {
				m.attaching = true
				m.attachPath = msg.path
				m.focus = focusAttachment
			}
			m.setToast("Gagal mengirim lampiran: "+msg.err.Error(), true)
			return m, nil
		}
		m.refreshChats()
		if m.compose == msg.caption {
			m.compose = ""
		}
		m.reply = nil
		if msg.err != nil {
			m.setToast("Lampiran terkirim, tetapi gagal menyimpan cache: "+msg.err.Error(), true)
		} else {
			m.setToast("Lampiran terkirim", false)
		}
		return m, nil

	case outboxResultMsg:
		m.refreshChats()
		if msg.err != nil {
			m.setToast(fmt.Sprintf("Outbox: %d terkirim; sisanya perlu review: %v", msg.sent, msg.err), true)
		} else {
			m.setToast(fmt.Sprintf("Outbox: %d pesan terkirim", msg.sent), false)
		}
		return m, nil

	case sendResultMsg:
		if !msg.sent {
			if errors.Is(msg.err, ErrOutboxQueued) {
				m.setToast("Offline: pesan disimpan di outbox. Ctrl+O untuk retry saat online.", false)
				return m, nil
			}
			m.restoreDraft(msg)
			m.setToast("Gagal mengirim: "+msg.err.Error(), true)
			return m, nil
		}
		m.scrollOffset = 0
		m.refreshChats()
		if msg.err != nil {
			m.setToast("Pesan terkirim, tetapi gagal menyimpan cache: "+msg.err.Error(), true)
		} else {
			m.setToast("Pesan terkirim", false)
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *tuiModel) restoreDraft(msg sendResultMsg) {
	if m.selectedJID != msg.jid {
		if m.pendingDrafts == nil {
			m.pendingDrafts = make(map[string]sendResultMsg)
		}
		if previous, ok := m.pendingDrafts[msg.jid]; ok {
			msg.text += "\n" + previous.text
		}
		m.pendingDrafts[msg.jid] = msg
		return
	}
	if m.compose == "" {
		m.compose = msg.text
	} else if msg.text != "" {
		m.compose = msg.text + "\n" + m.compose
	}
	m.reply = msg.reply
}

func (m *tuiModel) restorePendingDraft() {
	msg, ok := m.pendingDrafts[m.selectedJID]
	if !ok {
		return
	}
	delete(m.pendingDrafts, m.selectedJID)
	m.restoreDraft(msg)
}

func (m tuiModel) View() string {
	width := m.width
	if width < 76 {
		width = 76
	}
	height := m.height
	if height < 18 {
		height = 18
	}

	leftWidth := width / 3
	if leftWidth < 25 {
		leftWidth = 25
	}
	if leftWidth > 35 {
		leftWidth = 35
	}
	rightWidth := width - leftWidth - 3
	contentTop := 3
	footerDivider := height - 5
	contentHeight := footerDivider - contentTop
	if contentHeight < 3 {
		contentHeight = 3
	}

	rows := make([]string, 0, height)
	rows = append(rows, m.renderHeader(width))
	rows = append(rows, m.renderSubheader(width))
	rows = append(rows, strings.Repeat("─", width))

	leftRows := m.renderChatList(leftWidth, contentHeight)
	rightRows := m.renderConversation(rightWidth, contentHeight)
	for i := 0; i < contentHeight; i++ {
		left := ""
		right := ""
		if i < len(leftRows) {
			left = leftRows[i]
		}
		if i < len(rightRows) {
			right = rightRows[i]
		}
		rows = append(rows, fitANSI(left, leftWidth)+" │ "+fitANSI(right, rightWidth))
	}

	rows = append(rows, strings.Repeat("─", width))
	rows = append(rows, m.renderComposer(width))
	rows = append(rows, fitANSI("  Enter: kirim  ·  Ctrl+F: cari  ·  Ctrl+N: chat baru  ·  ↑↓: chat  ·  PgUp/PgDn: riwayat  ·  Ctrl+C: keluar", width))
	status := m.renderStatus(width)
	rows = append(rows, status)

	if len(rows) > height {
		rows = rows[:height]
	}
	for len(rows) < height {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

func (m tuiModel) renderHeader(width int) string {
	brand := ansiBold + ansiCyan + "  ✦ WhatsFuwa" + ansiReset + "  "
	status := ansiGreen + "● TERHUBUNG" + ansiReset
	if !m.ui.wa.IsConnected() {
		status = ansiYellow + "● MENYAMBUNG" + ansiReset
	}
	actions := "[ + New ] [ / Search ] [ × Exit ]"
	plainLeft := "  ✦ WhatsFuwa  "
	if m.ui.wa.IsConnected() {
		plainLeft += "● TERHUBUNG"
	} else {
		plainLeft += "● MENYAMBUNG"
	}
	gap := width - visibleLen(plainLeft) - visibleLen(actions)
	if gap < 2 {
		gap = 2
	}
	return brand + status + strings.Repeat(" ", gap) + ansiCyan + actions + ansiReset
}

func (m tuiModel) renderSubheader(width int) string {
	left := fmt.Sprintf("  %sCHATS%s  %d", ansiBold, ansiReset, len(m.chats))
	right := "  CONVERSATION"
	if m.attaching {
		right = "  ATTACHMENT"
	} else if m.searchOpen {
		right = "  SEARCH  / " + m.filter + "▏"
	} else if m.creating {
		right = "  NEW CONVERSATION"
	} else if chat, ok := m.selectedChat(); ok {
		right = "  " + chatTitle(chat)
	} else {
		right = "  PILIH PERCAKAPAN"
	}
	leftWidth := leftWidthFor(width)
	return fitANSI(left, leftWidth) + strings.Repeat(" ", 3) + fitANSI(right, width-leftWidth-3)
}

func (m tuiModel) renderChatList(width, height int) []string {
	rows := make([]string, 0, height)
	chats := m.visibleChats()
	if len(chats) == 0 {
		if m.filter != "" {
			rows = append(rows, ansiDim+"  Tidak ada hasil."+ansiReset)
		} else {
			rows = append(rows, ansiDim+"  Belum ada chat."+ansiReset)
			rows = append(rows, ansiDim+"  Klik + New untuk"+ansiReset)
			rows = append(rows, ansiDim+"  memulai chat."+ansiReset)
		}
		return padRows(rows, height)
	}

	start := maxInt(0, m.listOffset)
	visibleCount := (height + 1) / 2
	start = minInt(start, maxInt(0, len(chats)-visibleCount))
	end := minInt(len(chats), start+visibleCount)
	for index := start; index < end; index++ {
		chat := chats[index]
		name := chatTitle(chat)
		preview := chat.LastMessage
		if preview == "" {
			preview = "Belum ada pesan"
		}
		marker := "  "
		style := ""
		if chat.JID == m.selectedJID {
			marker = "▸ "
			style = ansiSelect
		}
		badge := ""
		if chat.Unread > 0 {
			badge = fmt.Sprintf(" [%d]", chat.Unread)
		}
		first := marker + truncate(name, width-13-len(badge)) + "  " + chat.UpdatedAt.Local().Format("15:04") + badge
		second := "  " + ansiDim + truncate(preview, width-4) + ansiReset
		if style != "" {
			first = style + first + ansiReset
			second = style + second + ansiReset
		}
		rows = append(rows, first, second)
	}
	return padRows(rows, height)
}

func (m tuiModel) renderConversation(width, height int) []string {
	if m.attaching {
		rows := []string{
			"",
			ansiBold + "  Kirim lampiran" + ansiReset,
			"",
			"  Masukkan path file yang ingin dikirim.",
			"  Ukuran maksimum 64 MiB. Caption memakai teks di composer.",
			"",
			ansiDim + "  Enter: kirim · Esc: batal" + ansiReset,
		}
		return padRows(rows, height)
	}
	if m.creating {
		rows := []string{
			"",
			ansiBold + "  Mulai percakapan baru" + ansiReset,
			"",
			"  Masukkan nomor WhatsApp dalam format internasional.",
			"  Contoh: 628123456789",
			"",
			ansiDim + "  Tekan Enter atau klik Create untuk melanjutkan." + ansiReset,
		}
		return padRows(rows, height)
	}
	chat, ok := m.selectedChat()
	if !ok {
		rows := []string{
			"",
			ansiBold + ansiCyan + "  Selamat datang di WhatsFuwa" + ansiReset,
			"",
			"  WhatsApp yang ringan, langsung dari terminal.",
			"  Pilih chat di panel kiri atau klik + New.",
			"",
			ansiDim + "  Tidak ada Chromium. Tidak ada browser berat." + ansiReset,
		}
		return padRows(rows, height)
	}

	messageLines := conversationLines(chat, width, m.selectedMessageID)
	maxOffset := maxConversationOffset(len(messageLines), height)
	offset := minInt(maxOffset, maxInt(0, m.scrollOffset))
	visibleHeight := height
	if offset > 0 {
		visibleHeight = maxInt(1, height-1)
	}
	end := len(messageLines) - offset
	start := maxInt(0, end-visibleHeight)
	visible := append([]string(nil), messageLines[start:end]...)
	if offset > 0 {
		visible = append([]string{ansiDim + "  ↑ Riwayat pesan lebih lama (PgUp / scroll)" + ansiReset}, visible...)
	}
	return padRows(visible, height)
}

func conversationLines(chat Chat, width int, selectedMessageID string) []string {
	messageLines := make([]string, 0, len(chat.Messages)*3)
	for _, message := range chat.Messages {
		name := message.Sender
		if message.FromMe {
			name = "Kamu"
		} else if name == "" || name == chat.JID {
			name = chatTitle(chat)
		}
		timestamp := message.Timestamp.Local().Format("15:04")
		if message.Edited {
			timestamp += " · diedit"
		}
		header := ""
		if message.FromMe {
			header = ansiCyan + "  Kamu  ·  " + timestamp + deliveryLabel(message.Delivery) + ansiReset
		} else {
			header = ansiBold + "  " + truncate(name, maxInt(8, width-10)) + ansiReset + ansiDim + "  ·  " + timestamp + ansiReset
		}
		if message.ID == selectedMessageID {
			header = ansiSelect + "▸ " + header + ansiReset
		}
		messageLines = append(messageLines, header)
		if message.ReplyToID != "" {
			quoted := previewText(message.ReplyPreview)
			if quoted == "" {
				quoted = "pesan sebelumnya"
			}
			messageLines = append(messageLines, ansiDim+"  ↪ "+truncate(quoted, maxInt(12, width-8))+ansiReset)
		}
		if message.Deleted {
			messageLines = append(messageLines, ansiDim+"  [Pesan dihapus]"+ansiReset)
		} else {
			wrapped := wrapText(message.Text, maxInt(12, width-5))
			for _, line := range wrapped {
				messageLines = append(messageLines, "  "+line)
			}
		}
		if reactions := reactionSummary(message.Reactions); reactions != "" {
			messageLines = append(messageLines, "  "+reactions)
		}
		messageLines = append(messageLines, "")
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "", "  Belum ada pesan yang tersimpan.", "  Tulis pesan di bawah untuk memulai.")
	}
	return messageLines
}

func deliveryLabel(status DeliveryStatus) string {
	switch status {
	case DeliverySent:
		return " · ✓"
	case DeliveryDelivered:
		return " · ✓✓"
	case DeliveryRead:
		return " · ✓✓ dibaca"
	case DeliveryPlayed:
		return " · dimainkan"
	default:
		return ""
	}
}

func reactionSummary(reactions map[string]string) string {
	counts := make(map[string]int)
	for _, emoji := range reactions {
		if emoji != "" {
			counts[emoji]++
		}
	}
	ordered := make([]string, 0, len(counts))
	for emoji := range counts {
		ordered = append(ordered, emoji)
	}
	sort.Strings(ordered)
	parts := make([]string, 0, len(ordered))
	for _, emoji := range ordered {
		parts = append(parts, fmt.Sprintf("%s %d", emoji, counts[emoji]))
	}
	return strings.Join(parts, " · ")
}

func (m tuiModel) maxScrollOffset() int {
	chat, ok := m.selectedChat()
	if !ok {
		return 0
	}
	width := maxInt(76, m.width)
	height := maxInt(3, maxInt(18, m.height)-8)
	lines := conversationLines(chat, width-leftWidthFor(width)-3, m.selectedMessageID)
	return maxConversationOffset(len(lines), height)
}

func maxConversationOffset(lineCount, height int) int {
	if lineCount <= height {
		return 0
	}
	return lineCount - height + 1
}

func (m *tuiModel) scrollBy(delta int) {
	m.scrollOffset = minInt(m.maxScrollOffset(), maxInt(0, m.scrollOffset+delta))
}

func (m tuiModel) renderComposer(width int) string {
	label := "  > "
	value := m.compose
	button := "[ Send ]"
	if m.attaching {
		label = "  File > "
		value = m.attachPath
		button = "[ Attach ]"
	} else if m.creating {
		label = "  Nomor > "
		value = m.newChat
		button = "[ Create ]"
	} else if reply := m.activeReply(); reply != nil {
		label = "  ↪ " + truncate(previewText(reply.Text), 24) + " > "
	}
	if m.focus == focusComposer || m.focus == focusNewChat || m.focus == focusAttachment {
		label = ansiCyan + label + ansiReset
	}
	available := width - visibleLen(label) - len(button) - 3
	if available < 8 {
		available = 8
	}
	placeholder := "Tulis pesan..."
	if m.attaching {
		placeholder = "/path/file.png"
	} else if m.creating {
		placeholder = "628123456789"
	}
	if value == "" {
		value = ansiDim + placeholder + ansiReset
	} else {
		value = truncate(value, available)
	}
	space := width - visibleLen(label) - visibleLen(value) - len(button) - 2
	if space < 1 {
		space = 1
	}
	return label + value + strings.Repeat(" ", space) + ansiCyan + button + ansiReset + " "
}

func (m tuiModel) renderStatus(width int) string {
	message := "  WhatsFuwa · Go + whatsmeow · klik chat untuk membuka"
	if m.ui != nil && m.ui.store != nil && len(m.ui.store.Outbox()) > 0 {
		message = fmt.Sprintf("  Outbox: %d pesan · Ctrl+O retry manual", len(m.ui.store.Outbox()))
	}
	if m.focus == focusMessage {
		message = "  Pesan: r reply · c copy · d download · v preview · o buka · Esc kembali"
	}
	if m.toast != "" {
		color := ansiGreen
		if m.toastIsErr {
			color = ansiRed
		}
		message = "  " + color + sanitizeText(m.toast) + ansiReset
	}
	return fitANSI(ansiDim+message+ansiReset, width)
}

func (m tuiModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	width := maxInt(76, m.width)
	height := maxInt(18, m.height)
	leftWidth := width / 3
	leftWidth = minInt(35, maxInt(25, leftWidth))
	contentTop := 3
	footerDivider := height - 5
	contentHeight := footerDivider - contentTop
	if contentHeight < 3 {
		contentHeight = 3
	}
	actions := "[ + New ] [ / Search ] [ × Exit ]"
	actionStart := width - visibleLen(actions)

	if msg.Button == tea.MouseButtonWheelUp {
		if msg.X < leftWidth && msg.Y >= contentTop && msg.Y < footerDivider {
			m.listOffset = maxInt(0, m.listOffset-2)
			return m, nil
		}
		if msg.X >= leftWidth && msg.Y >= contentTop && msg.Y < footerDivider {
			m.scrollBy(3)
			return m, nil
		}
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if msg.X < leftWidth && msg.Y >= contentTop && msg.Y < footerDivider {
			visibleCount := (contentHeight + 1) / 2
			maxOffset := maxInt(0, len(m.visibleChats())-visibleCount)
			m.listOffset = minInt(maxOffset, m.listOffset+2)
			return m, nil
		}
		if msg.X >= leftWidth && msg.Y >= contentTop && msg.Y < footerDivider {
			m.scrollBy(-3)
			return m, nil
		}
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if msg.Y == 0 && msg.X >= actionStart {
		relative := msg.X - actionStart
		switch {
		case relative < len("[ + New ]"):
			m.beginNewChat()
		case relative < len("[ + New ] [ / Search ]"):
			m.beginSearch()
		default:
			return m, tea.Quit
		}
		return m, nil
	}
	if msg.Y >= contentTop && msg.Y < footerDivider && msg.X < leftWidth {
		chats := m.visibleChats()
		visibleCount := (contentHeight + 1) / 2
		maxOffset := maxInt(0, len(chats)-visibleCount)
		m.listOffset = minInt(maxOffset, maxInt(0, m.listOffset))
		index := m.listOffset + (msg.Y-contentTop)/2
		if index >= 0 && index < len(chats) {
			m.selectedJID = chats[index].JID
			m.selectedMessageID = ""
			m.creating = false
			m.newChat = ""
			m.attaching = false
			m.attachPath = ""
			m.searchOpen = false
			m.filter = ""
			m.scrollOffset = 0
			m.focus = focusComposer
			m.refreshChats()
		}
		return m, nil
	}
	if msg.Y == footerDivider+1 {
		if msg.X >= width-len("[ Attach ]")-2 && m.attaching {
			return m.sendAttachmentCurrent()
		}
		if msg.X >= width-len("[ Create ]")-2 && m.creating {
			m.createChat()
			return m, nil
		}
		if msg.X >= width-len("[ Send ]")-2 && !m.creating {
			return m.sendCurrent()
		}
		if m.creating {
			m.focus = focusNewChat
		} else {
			m.focus = focusComposer
		}
	}
	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	// The space bar arrives as its own key type rather than as a rune, so
	// normalize it here; otherwise spaces are silently dropped while typing.
	if msg.Type == tea.KeySpace {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	}
	switch msg.String() {
	case "ctrl+f":
		m.beginSearch()
		return m, nil
	case "ctrl+n":
		m.beginNewChat()
		return m, nil
	case "ctrl+g":
		m.startMessageSelection()
		return m, nil
	case "ctrl+h":
		return m.requestHistory()
	case "ctrl+a":
		m.beginAttachment()
		return m, nil
	case "ctrl+o":
		return m.retryOutbox()
	}
	if msg.Type == tea.KeyEsc {
		if m.attaching || m.focus == focusAttachment {
			m.attaching = false
			m.attachPath = ""
			m.focus = focusComposer
			return m, nil
		}
		if m.focus == focusMessage {
			m.focus = focusComposer
			m.selectedMessageID = ""
			return m, nil
		}
		if m.creating {
			m.creating = false
			m.newChat = ""
			m.focus = focusComposer
		} else if m.searchOpen {
			m.searchOpen = false
			m.filter = ""
			m.refreshChats()
			m.focus = focusList
		} else if m.reply != nil {
			m.reply = nil
			m.setToast("Balasan dibatalkan", false)
		} else {
			m.focus = focusComposer
		}
		return m, nil
	}

	if msg.Type == tea.KeyUp {
		if m.focus == focusMessage {
			m.moveMessageSelection(-1)
		} else if m.focus == focusComposer && m.compose == "" {
			m.moveSelection(-1)
		} else if m.focus == focusList {
			m.moveSelection(-1)
		}
		return m, nil
	}
	if msg.Type == tea.KeyDown {
		if m.focus == focusMessage {
			m.moveMessageSelection(1)
		} else if m.focus == focusComposer && m.compose == "" {
			m.moveSelection(1)
		} else if m.focus == focusList {
			m.moveSelection(1)
		}
		return m, nil
	}
	if msg.Type == tea.KeyPgUp {
		m.scrollBy(maxInt(3, m.height/2))
		return m, nil
	}
	if msg.Type == tea.KeyPgDown {
		m.scrollBy(-maxInt(3, m.height/2))
		return m, nil
	}
	if msg.Type == tea.KeyTab {
		if m.focus == focusList {
			m.focus = focusComposer
		} else {
			m.focus = focusList
		}
		return m, nil
	}

	if m.attaching || m.focus == focusAttachment {
		switch msg.Type {
		case tea.KeyEnter:
			return m.sendAttachmentCurrent()
		case tea.KeyBackspace:
			m.attachPath = trimLastRune(m.attachPath)
		case tea.KeyRunes:
			m.attachPath += string(msg.Runes)
		}
		return m, nil
	}

	if m.focus == focusMessage {
		if msg.Type == tea.KeyEnter || (msg.Type == tea.KeyRunes && string(msg.Runes) == "r") {
			m.replyToSelectedMessage()
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			switch string(msg.Runes) {
			case "c":
				message, ok := m.selectedMessage()
				if !ok {
					return m, nil
				}
				m.setToast("Teks disalin lewat clipboard terminal", false)
				return m, tea.Printf("%s", xansi.SetSystemClipboard(message.Text))
			case "d":
				return m.runMediaAction(mediaDownloadOnly)
			case "v":
				return m.runMediaAction(mediaPreview)
			case "o":
				return m.runMediaAction(mediaOpenExternal)
			}
		}
		return m, nil
	}

	if m.searchOpen || m.focus == focusSearch {
		switch msg.Type {
		case tea.KeyEnter:
			m.searchOpen = false
			m.focus = focusList
			m.refreshChats()
			return m, nil
		case tea.KeyBackspace:
			m.filter = trimLastRune(m.filter)
			m.refreshChats()
			return m, nil
		case tea.KeyRunes:
			m.filter += string(msg.Runes)
			m.refreshChats()
			return m, nil
		}
		return m, nil
	}

	if m.creating || m.focus == focusNewChat {
		switch msg.Type {
		case tea.KeyEnter:
			m.createChat()
		case tea.KeyBackspace:
			m.newChat = trimLastRune(m.newChat)
		case tea.KeyRunes:
			m.newChat += string(msg.Runes)
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+r":
		m.startReply()
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		return m.sendCurrent()
	case tea.KeyBackspace:
		m.compose = trimLastRune(m.compose)
	case tea.KeyRunes:
		m.compose += string(msg.Runes)
	}
	return m, nil
}

func (m tuiModel) requestHistory() (tea.Model, tea.Cmd) {
	chat, ok := m.selectedChat()
	if !ok || len(chat.Messages) == 0 {
		m.setToast("Belum ada pesan acuan untuk meminta riwayat", true)
		return m, nil
	}
	if m.historyPending[chat.JID] {
		m.setToast("Permintaan riwayat chat ini masih berjalan", false)
		return m, nil
	}
	if m.ui == nil || m.ui.wa == nil || m.ui.wa.Client == nil {
		m.setToast("WhatsFuwa belum terhubung", true)
		return m, nil
	}
	jid, err := types.ParseJID(chat.JID)
	if err != nil {
		m.setToast("Alamat chat tidak valid: "+err.Error(), true)
		return m, nil
	}
	if m.historyPending == nil {
		m.historyPending = make(map[string]bool)
	}
	m.historyPending[chat.JID] = true
	m.setToast("Meminta 50 pesan lama dari ponsel...", false)
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return m, func() tea.Msg {
		historyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		return historyResultMsg{jid: chat.JID, err: m.ui.wa.RequestHistory(historyCtx, jid)}
	}
}

func (m *tuiModel) beginAttachment() {
	if _, ok := m.selectedChat(); !ok {
		m.setToast("Pilih chat sebelum melampirkan file", true)
		return
	}
	m.attaching = true
	m.attachPath = ""
	m.creating = false
	m.searchOpen = false
	m.focus = focusAttachment
}

func (m tuiModel) sendAttachmentCurrent() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.attachPath)
	if len(path) >= 2 && ((path[0] == '"' && path[len(path)-1] == '"') || (path[0] == '\'' && path[len(path)-1] == '\'')) {
		path = path[1 : len(path)-1]
	}
	if path == "" {
		return m, nil
	}
	chat, ok := m.selectedChat()
	if !ok {
		m.setToast("Pilih chat terlebih dahulu", true)
		return m, nil
	}
	jid, err := types.ParseJID(chat.JID)
	if err != nil {
		m.setToast("Alamat chat tidak valid: "+err.Error(), true)
		return m, nil
	}
	if m.ui == nil || m.ui.wa == nil || m.ui.wa.Client == nil || !m.ui.wa.Client.IsConnected() {
		m.setToast("WhatsFuwa belum terhubung", true)
		return m, nil
	}
	caption := m.compose
	reply := m.activeReply()
	m.attaching = false
	m.attachPath = ""
	m.focus = focusComposer
	m.setToast("Mengunggah lampiran...", false)
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return m, func() tea.Msg {
		sendCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		sent, err := m.ui.wa.SendAttachment(sendCtx, jid, path, caption, reply)
		return attachmentResultMsg{jid: chat.JID, path: path, caption: caption, sent: sent, err: err}
	}
}

func (m tuiModel) retryOutbox() (tea.Model, tea.Cmd) {
	if m.ui == nil || m.ui.store == nil || len(m.ui.store.Outbox()) == 0 {
		m.setToast("Outbox kosong", false)
		return m, nil
	}
	if m.ui.wa == nil || m.ui.wa.Client == nil {
		m.setToast("WhatsFuwa belum terhubung", true)
		return m, nil
	}
	m.setToast("Mengirim ulang outbox...", false)
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return m, func() tea.Msg {
		retryCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		sent, err := m.ui.wa.RetryOutbox(retryCtx)
		return outboxResultMsg{sent: sent, err: err}
	}
}

func (m tuiModel) sendCurrent() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.compose)
	if text == "" {
		return m, nil
	}
	chat, ok := m.selectedChat()
	if !ok {
		m.setToast("Pilih chat terlebih dahulu", true)
		return m, nil
	}
	jid, err := types.ParseJID(chat.JID)
	if err != nil {
		m.setToast("Alamat chat tidak valid: "+err.Error(), true)
		return m, nil
	}
	if m.ui == nil || m.ui.wa == nil || m.ui.wa.Client == nil {
		m.setToast("WhatsFuwa belum terhubung", true)
		return m, nil
	}
	reply := m.activeReply()
	m.compose = ""
	m.reply = nil
	m.focus = focusComposer
	m.setToast("Mengirim pesan...", false)
	ctx := m.ctx
	return m, func() tea.Msg {
		sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		sent, err := m.ui.wa.SendText(sendCtx, jid, text, reply)
		return sendResultMsg{jid: chat.JID, text: text, reply: reply, sent: sent, err: err}
	}
}

func (m *tuiModel) createChat() {
	phone := digitsOnly(m.newChat)
	if len(phone) < 8 || strings.HasPrefix(phone, "0") {
		m.setToast("Gunakan nomor internasional, contoh 628123456789", true)
		return
	}
	jid := types.NewJID(phone, types.DefaultUserServer)
	cacheErr := m.ui.store.AddChat(jid.String(), "")
	m.selectedJID = jid.String()
	m.newChat = ""
	m.creating = false
	m.searchOpen = false
	m.filter = ""
	m.scrollOffset = 0
	m.focus = focusComposer
	m.refreshChats()
	if cacheErr != nil {
		m.setToast("Chat baru siap, tetapi gagal menyimpan cache: "+cacheErr.Error(), true)
		return
	}
	m.setToast("Chat baru siap. Tulis pesan di bawah.", false)
}

func (m *tuiModel) beginSearch() {
	m.creating = false
	m.newChat = ""
	m.attaching = false
	m.attachPath = ""
	m.searchOpen = true
	m.focus = focusSearch
}

func (m *tuiModel) beginNewChat() {
	m.creating = true
	m.newChat = ""
	m.attaching = false
	m.attachPath = ""
	m.searchOpen = false
	m.filter = ""
	m.focus = focusNewChat
	m.refreshChats()
}

func (m *tuiModel) markReadCmd() tea.Cmd {
	if m.readInFlight || time.Since(m.lastReadAttempt) < 15*time.Second || m.ui == nil || m.ui.wa == nil || m.ui.wa.Client == nil {
		return nil
	}
	chat, ok := m.selectedChat()
	if !ok || chat.Unread == 0 {
		return nil
	}
	jid, err := types.ParseJID(chat.JID)
	if err != nil {
		return nil
	}
	m.readInFlight = true
	m.lastReadAttempt = time.Now()
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return func() tea.Msg {
		readCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return readResultMsg{jid: chat.JID, err: m.ui.wa.MarkChatRead(readCtx, jid)}
	}
}

func (m *tuiModel) refreshChats() {
	defer m.restorePendingDraft()
	previous := m.selectedJID
	m.chats = m.ui.store.Find(m.filter)
	if len(m.chats) == 0 {
		m.selectedJID = ""
		m.selectedMessageID = ""
		m.selectedIdx = 0
		m.listOffset = 0
		return
	}
	for i, chat := range m.chats {
		if chat.JID == previous {
			m.selectedIdx = i
			m.selectedJID = previous
			m.keepListSelectionVisible()
			return
		}
	}
	m.selectedIdx = 0
	m.selectedJID = m.chats[0].JID
	m.selectedMessageID = ""
	m.listOffset = 0
}

func (m tuiModel) visibleChats() []Chat {
	if m.filter == "" {
		return m.chats
	}
	return m.ui.store.Find(m.filter)
}

func (m tuiModel) selectedChat() (Chat, bool) {
	for _, chat := range m.chats {
		if chat.JID == m.selectedJID {
			return chat, true
		}
	}
	return Chat{}, false
}

func (m *tuiModel) moveSelection(delta int) {
	chats := m.visibleChats()
	if len(chats) == 0 {
		return
	}
	index := m.selectedIdx + delta
	if index < 0 {
		index = 0
	}
	if index >= len(chats) {
		index = len(chats) - 1
	}
	m.selectedIdx = index
	m.selectedJID = chats[index].JID
	m.selectedMessageID = ""
	m.scrollOffset = 0
	m.keepListSelectionVisible()
}

func (m *tuiModel) keepListSelectionVisible() {
	contentHeight := maxInt(3, maxInt(18, m.height)-8)
	visible := maxInt(1, (contentHeight+1)/2)
	if m.selectedIdx < m.listOffset {
		m.listOffset = m.selectedIdx
	}
	if m.selectedIdx >= m.listOffset+visible {
		m.listOffset = m.selectedIdx - visible + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}

func (m *tuiModel) setToast(value string, isErr bool) {
	m.toast = value
	m.toastIsErr = isErr
}

func (m tuiModel) runMediaAction(action mediaAction) (tea.Model, tea.Cmd) {
	message, ok := m.selectedMessage()
	if !ok || message.MediaType == "" {
		m.setToast("Pesan yang dipilih bukan media", true)
		return m, nil
	}
	if message.MediaPath != "" {
		if info, err := os.Stat(message.MediaPath); err == nil && info.Mode().IsRegular() {
			return m, func() tea.Msg {
				return mediaResultMsg{message: message, action: action, path: message.MediaPath}
			}
		}
	}
	if m.ui == nil || m.ui.wa == nil || m.ui.wa.Client == nil {
		m.setToast("WhatsFuwa belum terhubung untuk mengunduh media", true)
		return m, nil
	}
	m.setToast("Mengunduh media...", false)
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return m, func() tea.Msg {
		downloadCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		path, err := m.ui.wa.DownloadMedia(downloadCtx, message)
		return mediaResultMsg{message: message, action: action, path: path, err: err}
	}
}

func (m tuiModel) previewMedia(path string) (tea.Model, tea.Cmd) {
	if DetectImageProtocol(os.Getenv) == ImageNone {
		return m.openMedia(path)
	}
	executable, err := os.Executable()
	if err != nil {
		m.setToast("Menemukan binary preview: "+err.Error(), true)
		return m, nil
	}
	command := exec.Command(executable, "media-preview", path)
	return m, tea.ExecProcess(command, func(err error) tea.Msg {
		return mediaOpenResultMsg{path: path, err: err}
	})
}

func (m tuiModel) openMedia(path string) (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", path)
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
		default:
			command = exec.Command("xdg-open", path)
		}
		if err := command.Start(); err != nil {
			return mediaOpenResultMsg{path: path, err: err}
		}
		_ = command.Process.Release()
		return mediaOpenResultMsg{path: path}
	}
}

func (m *tuiModel) startMessageSelection() {
	chat, ok := m.selectedChat()
	if !ok || len(chat.Messages) == 0 {
		m.setToast("Tidak ada pesan untuk dipilih", true)
		return
	}
	m.selectedMessageID = chat.Messages[len(chat.Messages)-1].ID
	m.scrollOffset = 0
	m.focus = focusMessage
	m.creating = false
	m.attaching = false
	m.attachPath = ""
	m.searchOpen = false
}

func (m *tuiModel) moveMessageSelection(delta int) {
	chat, ok := m.selectedChat()
	if !ok || len(chat.Messages) == 0 {
		return
	}
	index := len(chat.Messages) - 1
	for i, message := range chat.Messages {
		if message.ID == m.selectedMessageID {
			index = i
			break
		}
	}
	index = minInt(len(chat.Messages)-1, maxInt(0, index+delta))
	m.selectedMessageID = chat.Messages[index].ID
	m.keepSelectedMessageVisible(chat)
}

func (m *tuiModel) keepSelectedMessageVisible(chat Chat) {
	width := maxInt(76, m.width)
	height := maxInt(3, maxInt(18, m.height)-8)
	lines := conversationLines(chat, width-leftWidthFor(width)-3, m.selectedMessageID)
	selectedLine := -1
	for i, line := range lines {
		if strings.Contains(line, "▸ ") {
			selectedLine = i
			break
		}
	}
	if selectedLine < 0 {
		return
	}
	visibleHeight := height
	if m.scrollOffset > 0 {
		visibleHeight = maxInt(1, height-1)
	}
	end := len(lines) - m.scrollOffset
	start := maxInt(0, end-visibleHeight)
	if selectedLine >= end {
		m.scrollOffset = len(lines) - selectedLine - 1
	} else if selectedLine < start {
		m.scrollOffset = len(lines) - selectedLine - visibleHeight
	}
	m.scrollOffset = minInt(maxConversationOffset(len(lines), height), maxInt(0, m.scrollOffset))
}

func (m tuiModel) selectedMessage() (ChatMessage, bool) {
	chat, ok := m.selectedChat()
	if !ok {
		return ChatMessage{}, false
	}
	for _, message := range chat.Messages {
		if message.ID == m.selectedMessageID {
			return message, true
		}
	}
	return ChatMessage{}, false
}

func (m *tuiModel) replyToSelectedMessage() {
	message, ok := m.selectedMessage()
	if !ok {
		m.setToast("Pilih pesan terlebih dahulu", true)
		return
	}
	target := message
	m.reply = &target
	m.focus = focusComposer
	m.setToast("Membalas pesan. Esc untuk batal.", false)
}

// startReply makes the newest incoming message in the selected chat the
// target of the next sent message.
func (m *tuiModel) startReply() {
	chat, ok := m.selectedChat()
	if !ok {
		m.setToast("Pilih chat terlebih dahulu", true)
		return
	}
	for i := len(chat.Messages) - 1; i >= 0; i-- {
		message := chat.Messages[i]
		if message.FromMe || message.ID == "" {
			continue
		}
		target := message
		m.reply = &target
		m.selectedMessageID = target.ID
		m.focus = focusComposer
		m.setToast("Membalas pesan. Esc untuk batal.", false)
		return
	}
	m.setToast("Tidak ada pesan masuk untuk dibalas", true)
}

// activeReply returns the pending reply only when it belongs to the selected
// chat; switching chats silently drops it.
func (m tuiModel) activeReply() *ChatMessage {
	if m.reply == nil {
		return nil
	}
	chat, ok := m.selectedChat()
	if !ok || chat.JID != m.reply.ChatJID {
		return nil
	}
	return m.reply
}

// previewText collapses whitespace and newlines so a quote fits on one line.
func previewText(value string) string {
	return strings.Join(strings.Fields(sanitizeText(value)), " ")
}

func sanitizeText(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func scheduleRefresh() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return refreshMsg(now) })
}

func chatTitle(chat Chat) string {
	if chat.Title != "" {
		return sanitizeText(chat.Title)
	}
	return sanitizeText(chat.JID)
}

func digitsOnly(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func trimLastRune(value string) string {
	if value == "" {
		return value
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}

func truncate(value string, width int) string {
	value = sanitizeText(value)
	if width <= 0 {
		return ""
	}
	return runewidth.Truncate(value, width, "…")
}

func wrapText(value string, width int) []string {
	value = sanitizeText(value)
	if width < 1 {
		width = 1
	}
	var result []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := ""
		for _, word := range words {
			for runewidth.StringWidth(word) > width {
				if line != "" {
					result = append(result, line)
					line = ""
				}
				chunk := runewidth.Truncate(word, width, "")
				if chunk == "" {
					// A single wide grapheme cannot fit a one-cell viewport.
					chunk = word
				}
				result = append(result, chunk)
				word = word[len(chunk):]
			}
			if line == "" {
				line = word
				continue
			}
			if runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= width {
				line += " " + word
			} else {
				result = append(result, line)
				line = word
			}
		}
		result = append(result, line)
	}
	return result
}

func visibleLen(value string) int {
	return runewidth.StringWidth(ansiEscape.ReplaceAllString(value, ""))
}

func fitANSI(value string, width int) string {
	plain := ansiEscape.ReplaceAllString(value, "")
	visible := runewidth.StringWidth(plain)
	if visible > width {
		// Renderers interpret ANSI sequences; truncation is only used for the
		// footer and left column, so strip style rather than cut an escape code.
		return truncate(plain, width)
	}
	return value + strings.Repeat(" ", width-visible)
}

func padRows(rows []string, height int) []string {
	if len(rows) > height {
		return rows[:height]
	}
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

func leftWidthFor(width int) int {
	left := width / 3
	if left < 25 {
		return 25
	}
	if left > 35 {
		return 35
	}
	return left
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
