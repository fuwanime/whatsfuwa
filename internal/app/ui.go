package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
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

	chats        []Chat
	selectedJID  string
	selectedIdx  int
	listOffset   int
	scrollOffset int

	compose    string
	filter     string
	newChat    string
	searchOpen bool
	creating   bool
	focus      focusTarget

	toast      string
	toastIsErr bool
}

type refreshMsg time.Time

type sendResultMsg struct {
	jid  string
	text string
	err  error
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
		return m, scheduleRefresh()

	case sendResultMsg:
		if msg.err != nil {
			m.setToast("Gagal mengirim: "+msg.err.Error(), true)
		} else {
			m.setToast("Pesan terkirim", false)
			m.scrollOffset = 0
			m.refreshChats()
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
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
	if m.searchOpen {
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

	start := m.listOffset
	if start < 0 {
		start = 0
	}
	if start >= len(chats) {
		start = maxInt(0, len(chats)-1)
	}
	visibleCount := (height + 1) / 2
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
		first := marker + truncate(name, width-13) + "  " + chat.UpdatedAt.Local().Format("15:04")
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

	messageLines := make([]string, 0, len(chat.Messages)*3)
	for _, message := range chat.Messages {
		name := message.Sender
		if message.FromMe {
			name = "Kamu"
		} else if name == "" || name == chat.JID {
			name = chatTitle(chat)
		}
		timestamp := message.Timestamp.Local().Format("15:04")
		if message.FromMe {
			messageLines = append(messageLines, ansiCyan+"  Kamu  ·  "+timestamp+ansiReset)
		} else {
			messageLines = append(messageLines, ansiBold+"  "+truncate(name, maxInt(8, width-10))+ansiReset+ansiDim+"  ·  "+timestamp+ansiReset)
		}
		wrapped := wrapText(message.Text, maxInt(12, width-5))
		for _, line := range wrapped {
			messageLines = append(messageLines, "  "+line)
		}
		messageLines = append(messageLines, "")
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "", "  Belum ada pesan yang tersimpan.", "  Tulis pesan di bawah untuk memulai.")
	}

	end := len(messageLines) - m.scrollOffset
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	visible := append([]string(nil), messageLines[start:end]...)
	if m.scrollOffset > 0 {
		visible = append([]string{ansiDim + "  ↑ Riwayat pesan lebih lama (PgUp / scroll)" + ansiReset}, visible...)
		if len(visible) > height {
			visible = visible[len(visible)-height:]
		}
	}
	return padRows(visible, height)
}

func (m tuiModel) renderComposer(width int) string {
	label := "  > "
	value := m.compose
	button := "[ Send ]"
	if m.creating {
		label = "  Nomor > "
		value = m.newChat
		button = "[ Create ]"
	}
	if m.focus == focusComposer || m.focus == focusNewChat {
		label = ansiCyan + label + ansiReset
	}
	available := width - visibleLen(label) - len(button) - 3
	if available < 8 {
		available = 8
	}
	placeholder := "Tulis pesan..."
	if m.creating {
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
	if m.toast != "" {
		color := ansiGreen
		if m.toastIsErr {
			color = ansiRed
		}
		message = "  " + color + m.toast + ansiReset
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
			m.scrollOffset += 3
			return m, nil
		}
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if msg.X < leftWidth && msg.Y >= contentTop && msg.Y < footerDivider {
			m.listOffset = minInt(maxInt(0, len(m.visibleChats())-1), m.listOffset+2)
			return m, nil
		}
		if msg.X >= leftWidth && msg.Y >= contentTop && msg.Y < footerDivider {
			m.scrollOffset = maxInt(0, m.scrollOffset-3)
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
			m.searchOpen = true
			m.focus = focusSearch
		default:
			return m, tea.Quit
		}
		return m, nil
	}
	if msg.Y >= contentTop && msg.Y < footerDivider && msg.X < leftWidth {
		index := m.listOffset + (msg.Y-contentTop)/2
		chats := m.visibleChats()
		if index >= 0 && index < len(chats) {
			m.selectedJID = chats[index].JID
			m.selectedIdx = index
			m.scrollOffset = 0
			m.focus = focusComposer
		}
		return m, nil
	}
	if msg.Y == footerDivider+1 {
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
	if msg.Type == tea.KeyEsc {
		if m.creating {
			m.creating = false
			m.newChat = ""
			m.focus = focusComposer
		} else if m.searchOpen {
			m.searchOpen = false
			m.filter = ""
			m.refreshChats()
			m.focus = focusList
		} else {
			m.focus = focusComposer
		}
		return m, nil
	}

	if msg.Type == tea.KeyUp {
		if m.focus == focusComposer && m.compose == "" {
			m.moveSelection(-1)
		} else if m.focus == focusList {
			m.moveSelection(-1)
		}
		return m, nil
	}
	if msg.Type == tea.KeyDown {
		if m.focus == focusComposer && m.compose == "" {
			m.moveSelection(1)
		} else if m.focus == focusList {
			m.moveSelection(1)
		}
		return m, nil
	}
	if msg.Type == tea.KeyPgUp {
		m.scrollOffset += maxInt(3, m.height/2)
		return m, nil
	}
	if msg.Type == tea.KeyPgDown {
		m.scrollOffset = maxInt(0, m.scrollOffset-maxInt(3, m.height/2))
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
	case "ctrl+f":
		m.searchOpen = true
		m.focus = focusSearch
		return m, nil
	case "ctrl+n":
		m.beginNewChat()
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
	if !m.ui.wa.IsConnected() {
		m.setToast("WhatsFuwa belum terhubung", true)
		return m, nil
	}
	m.compose = ""
	m.focus = focusComposer
	m.setToast("Mengirim pesan...", false)
	ctx := m.ctx
	return m, func() tea.Msg {
		sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		err := m.ui.wa.SendText(sendCtx, jid, text)
		return sendResultMsg{jid: chat.JID, text: text, err: err}
	}
}

func (m *tuiModel) createChat() {
	phone := digitsOnly(m.newChat)
	if len(phone) < 8 || strings.HasPrefix(phone, "0") {
		m.setToast("Gunakan nomor internasional, contoh 628123456789", true)
		return
	}
	jid := types.NewJID(phone, types.DefaultUserServer)
	m.ui.store.AddChat(jid.String(), "")
	m.selectedJID = jid.String()
	m.newChat = ""
	m.creating = false
	m.searchOpen = false
	m.filter = ""
	m.scrollOffset = 0
	m.focus = focusComposer
	m.refreshChats()
	m.setToast("Chat baru siap. Tulis pesan di bawah.", false)
}

func (m *tuiModel) beginNewChat() {
	m.creating = true
	m.newChat = ""
	m.searchOpen = false
	m.focus = focusNewChat
}

func (m *tuiModel) refreshChats() {
	previous := m.selectedJID
	m.chats = m.ui.store.Find(m.filter)
	if len(m.chats) == 0 {
		m.selectedJID = ""
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
	m.scrollOffset = 0
	m.keepListSelectionVisible()
}

func (m *tuiModel) keepListSelectionVisible() {
	visible := maxInt(1, (maxInt(18, m.height)-8)/2)
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

func scheduleRefresh() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return refreshMsg(now) })
}

func chatTitle(chat Chat) string {
	if chat.Title != "" {
		return chat.Title
	}
	return chat.JID
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
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func wrapText(value string, width int) []string {
	if width < 8 {
		width = 8
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
			if line == "" {
				line = truncate(word, width)
				continue
			}
			if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width {
				line += " " + word
			} else {
				result = append(result, line)
				line = truncate(word, width)
			}
		}
		result = append(result, line)
	}
	return result
}

func visibleLen(value string) int {
	return utf8.RuneCountInString(ansiEscape.ReplaceAllString(value, ""))
}

func fitANSI(value string, width int) string {
	plain := ansiEscape.ReplaceAllString(value, "")
	visible := utf8.RuneCountInString(plain)
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
