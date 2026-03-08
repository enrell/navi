package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"navi/internal/config"
	"navi/internal/core/domain"
	orchestratorsvc "navi/internal/core/services/orchestrator"
	"navi/internal/telemetry"
)

const (
	defaultContextLimit    = 128000
	compactionThresholdPct = 0.70
	historyLimit           = 200
	fileSuggestionLimit    = 5
	fileDiscoveryLimit     = 2000
	minMarkdownWrapWidth   = 24
	defaultInputHeight     = 1
	maxInputHeight         = 5
)

type paletteCommand struct {
	Section  string
	Title    string
	Shortcut string
	Summary  string
	Action   string
}

type Orchestrator interface {
	AskWithTrace(ctx context.Context, userMessage string) (string, []orchestratorsvc.TraceEvent, error)
}

type Chatter interface {
	Chat(ctx context.Context, userMessage string) (string, error)
}

type AgentLister interface {
	List(ctx context.Context) ([]*domain.Agent, error)
}

type Services struct {
	Orchestrator Orchestrator
	Chat         Chatter
	Agents       AgentLister
	ModelName    string
	WorkDir      string
	ContextLimit int
}

type entryKind int

const (
	entrySystem entryKind = iota
	entryUser
	entryThinking
	entryTool
	entryAssistant
	entryError
)

type historyStore struct {
	Prompts []string `json:"prompts"`
}

type conversationTurn struct {
	User      string
	Assistant string
}

type transcriptEntry struct {
	Kind        entryKind
	Title       string
	Body        string
	Collapsible bool
	Timestamp   time.Time
}

type mentionState struct {
	Active      bool
	Query       string
	Suggestions []string
}

type responseMsg struct {
	Prompt   string
	Reply    string
	Trace    []orchestratorsvc.TraceEvent
	Err      error
	Duration time.Duration
	SentText string
	Canceled bool
}

type agentListMsg struct {
	Agents []string
	Err    error
}

type model struct {
	ctx               context.Context
	services          Services
	spinner           spinner.Model
	textarea          textarea.Model
	viewport          viewport.Model
	historyPath       string
	history           []string
	historyIndex      int
	workspaceFiles    []string
	agents            []string
	entries           []transcriptEntry
	turns             []conversationTurn
	summary           string
	showToolLogs      bool
	pending           bool
	requestCancel     context.CancelFunc
	mention           mentionState
	width             int
	height            int
	lastErr           error
	lastDuration      time.Duration
	compactedNotice   string
	compactedAt       time.Time
	renderer          *glamour.TermRenderer
	rendererWrapWidth int
	hiddenLogCount    int
	paletteOpen       bool
	paletteIndex      int
	paletteQuery      string
}

var errStopWalk = errors.New("stop walk")

func Run(ctx context.Context, in io.Reader, out io.Writer, services Services) error {
	services = normalizeServices(services)
	if services.Orchestrator == nil && services.Chat == nil {
		return fmt.Errorf("tui: neither orchestrator nor chat service is wired")
	}
	workDir := strings.TrimSpace(services.WorkDir)
	if workDir == "" {
		if wd, err := os.Getwd(); err == nil {
			workDir = wd
		}
	}
	services.WorkDir = workDir
	if services.ContextLimit <= 0 {
		services.ContextLimit = defaultContextLimit
	}

	historyPath, _ := defaultHistoryPath()
	history := loadHistory(historyPath)
	files := discoverWorkspaceFiles(workDir, fileDiscoveryLimit)

	ta := textarea.New()
	ta.Placeholder = "Ask Navi anything. Use @file to add context."
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetHeight(defaultInputHeight)
	ta.CharLimit = 0
	ta.MaxHeight = maxInputHeight
	ta.EndOfBufferCharacter = ' '
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+j", "alt+enter", "ctrl+enter"),
		key.WithHelp("ctrl+j", "insert newline"),
	)
	ta.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true

	m := model{
		ctx:            ctx,
		services:       services,
		spinner:        sp,
		textarea:       ta,
		viewport:       vp,
		historyPath:    historyPath,
		history:        history,
		historyIndex:   len(history),
		workspaceFiles: files,
		showToolLogs:   false,
	}
	m.appendEntry(entrySystem, "Session", "Navi orchestrator ready. Enter sends, Ctrl+J adds a new line, Ctrl+P opens commands, Ctrl+T toggles tool logs.", false)
	m.refreshViewport()

	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(in), tea.WithOutput(out))
	finalModel, err := program.Run()
	if fm, ok := finalModel.(model); ok && fm.renderer != nil {
		_ = fm.renderer.Close()
	}
	return err
}

func IsInteractiveTTY(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok {
		return false
	}
	outFile, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func normalizeServices(services Services) Services {
	if interfaceIsNil(services.Orchestrator) {
		services.Orchestrator = nil
	}
	if interfaceIsNil(services.Chat) {
		services.Chat = nil
	}
	if interfaceIsNil(services.Agents) {
		services.Agents = nil
	}
	return services
}

func interfaceIsNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.textarea.Focus())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.refreshViewport()
		return m, nil

	case spinner.TickMsg:
		if m.pending {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case responseMsg:
		m.pending = false
		m.lastDuration = msg.Duration
		m.requestCancel = nil

		if msg.Canceled || errors.Is(msg.Err, context.Canceled) {
			m.appendEntry(entrySystem, "Canceled", "The current action was interrupted and the conversation state was preserved.", false)
			m.refreshViewport()
			return m, nil
		}

		if msg.Err != nil {
			m.lastErr = msg.Err
			m.appendEntry(entryError, "Error", msg.Err.Error(), false)
			m.refreshViewport()
			return m, nil
		}

		for _, event := range msg.Trace {
			switch event.Type {
			case orchestratorsvc.TraceThinking:
				m.appendEntry(entryThinking, "Thinking", event.Content, true)
			case orchestratorsvc.TraceToolResponse:
				title := "Tool"
				if strings.TrimSpace(event.Tool) != "" {
					title = "Tool: " + strings.TrimSpace(event.Tool)
				}
				m.appendEntry(entryTool, title, event.Content, true)
			case orchestratorsvc.TraceOrchestrator:
				// Final reply is appended below so the ordering remains consistent.
			}
		}

		m.turns = append(m.turns, conversationTurn{User: msg.Prompt, Assistant: msg.Reply})
		m.summary, m.turns, m.compactedNotice = compactTurns(m.summary, m.turns, m.services.ContextLimit)
		if m.compactedNotice != "" {
			m.compactedAt = time.Now()
			m.appendEntry(entrySystem, "Context", m.compactedNotice, false)
		}
		m.appendEntry(entryAssistant, "Navi", msg.Reply, false)
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		if m.paletteOpen {
			return m.updatePalette(msg)
		}

		switch msg.String() {
		case "ctrl+c":
			if m.pending && m.requestCancel != nil {
				m.requestCancel()
				m.pending = false
				m.requestCancel = nil
				m.appendEntry(entrySystem, "Canceled", "Interrupt requested. Waiting for the current call to return.", false)
				m.refreshViewport()
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+l":
			m.entries = nil
			m.hiddenLogCount = 0
			m.appendEntry(entrySystem, "Session", "Transcript cleared. Session summary and history remain available for the next turn.", false)
			m.refreshViewport()
			return m, nil
		case "ctrl+t":
			m.showToolLogs = !m.showToolLogs
			m.refreshViewport()
			return m, nil
		case "ctrl+p":
			m.paletteOpen = true
			m.paletteIndex = 0
			m.paletteQuery = ""
			m.refreshViewport()
			return m, nil
		case "enter":
			cmd := m.submit()
			m.refreshViewport()
			return m, cmd
		case "tab":
			if m.completeMention() {
				m.refreshViewport()
				return m, nil
			}
		case "alt+up":
			m.historyBack()
			m.refreshMention()
			m.refreshViewport()
			return m, nil
		case "alt+down":
			m.historyForward()
			m.refreshMention()
			m.refreshViewport()
			return m, nil
		case "ctrl+s":
			cmd := m.submit()
			m.refreshViewport()
			return m, cmd
		case "esc":
			m.mention = mentionState{}
			m.refreshViewport()
			return m, nil
		}
	}

	var taCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)
	m.syncTextareaHeight()

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	m.refreshMention()
	m.refreshViewport()
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading Navi TUI..."
	}

	header := m.renderHeader()
	status := m.renderStatusBar()
	transcript := m.renderMainPane()
	input := m.renderInput()

	return lipgloss.JoinVertical(lipgloss.Left, header, transcript, input, status)
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	inputHeight := m.textarea.Height() + 4
	headerHeight := 1
	statusHeight := 1
	viewportHeight := m.height - inputHeight - headerHeight - statusHeight
	if viewportHeight < 6 {
		viewportHeight = 6
	}
	contentWidth := maxInt(m.width, 20)
	m.textarea.SetWidth(maxInt(contentWidth-8, 16))
	m.syncTextareaHeight()
	m.viewport.Width = contentWidth
	m.viewport.Height = viewportHeight
	if err := m.ensureRenderer(maxInt(contentWidth-10, minMarkdownWrapWidth)); err == nil {
		m.refreshViewport()
	}
}

func (m *model) ensureRenderer(wrapWidth int) error {
	if wrapWidth <= 0 {
		wrapWidth = minMarkdownWrapWidth
	}
	if m.renderer != nil && m.rendererWrapWidth == wrapWidth {
		return nil
	}
	if m.renderer != nil {
		_ = m.renderer.Close()
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(wrapWidth))
	if err != nil {
		return err
	}
	m.renderer = renderer
	m.rendererWrapWidth = wrapWidth
	return nil
}

func (m *model) submit() tea.Cmd {
	if m.pending {
		return nil
	}
	prompt := strings.TrimSpace(m.textarea.Value())
	if prompt == "" {
		return nil
	}

	sentText := buildPrompt(prompt, m.summary, m.turns)
	m.pending = true
	m.lastErr = nil
	m.appendEntry(entryUser, "You", prompt, false)
	m.appendHistory(prompt)
	m.textarea.Reset()
	m.syncTextareaHeight()
	m.refreshMention()

	ctx, cancel := context.WithCancel(m.ctx)
	m.requestCancel = cancel
	return tea.Batch(m.spinner.Tick, requestCmd(ctx, m.services, prompt, sentText))
}

func requestCmd(ctx context.Context, services Services, prompt string, sentText string) tea.Cmd {
	return func() tea.Msg {
		started := time.Now()
		if services.Orchestrator != nil {
			reply, trace, err := services.Orchestrator.AskWithTrace(ctx, sentText)
			return responseMsg{Prompt: prompt, Reply: reply, Trace: trace, Err: err, Duration: time.Since(started), SentText: sentText, Canceled: errors.Is(err, context.Canceled)}
		}
		reply, err := services.Chat.Chat(ctx, sentText)
		return responseMsg{Prompt: prompt, Reply: reply, Err: err, Duration: time.Since(started), SentText: sentText, Canceled: errors.Is(err, context.Canceled)}
	}
}

func loadAgentsCmd(agents AgentLister) tea.Cmd {
	return func() tea.Msg {
		if interfaceIsNil(agents) {
			return agentListMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		list, err := agents.List(ctx)
		if err != nil {
			return agentListMsg{Err: err}
		}
		ids := make([]string, 0, len(list))
		for _, agent := range list {
			if agent == nil || strings.TrimSpace(agent.ID) == "" {
				continue
			}
			ids = append(ids, agent.ID)
		}
		sort.Strings(ids)
		return agentListMsg{Agents: ids}
	}
}

func (m *model) refreshViewport() {
	if m.viewport.Width == 0 {
		return
	}
	parts := make([]string, 0, len(m.entries)+1)
	hidden := 0
	for _, entry := range m.entries {
		if entry.Collapsible && !m.showToolLogs {
			hidden++
			continue
		}
		parts = append(parts, m.renderEntry(entry))
	}
	if hidden > 0 {
		parts = append(parts, infoBoxStyle().Render(fmt.Sprintf("Tool logs hidden: %d entries. Press Ctrl+T to expand.", hidden)))
	}
	m.hiddenLogCount = hidden
	m.viewport.SetContent(strings.Join(parts, "\n\n"))
	m.viewport.GotoBottom()
}

func (m *model) renderEntry(entry transcriptEntry) string {
	body := strings.TrimSpace(m.renderBody(entry))
	if body == "" {
		body = "(empty)"
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(entryColor(entry.Kind)).
		Padding(0, 1).
		Width(maxInt(m.viewport.Width-2, 20))

	if entry.Kind == entryThinking && !strings.Contains(body, "\n") && lipgloss.Width(body) < maxInt(m.viewport.Width-18, 20) {
		label := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("180")).Render("Thinking:")
		text := lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Render(" " + body)
		return panel.Render(label + text)
	}

	header := renderEntryHeader(entry)
	return panel.Render(header + "\n" + body)
}

func (m *model) renderBody(entry transcriptEntry) string {
	body := strings.TrimSpace(entry.Body)
	if body == "" {
		return ""
	}
	if diff, ok := extractStandaloneDiff(body); ok {
		return renderDiff(diff)
	}
	if looksLikeJSON(body) {
		body = "```json\n" + body + "\n```"
	}
	if m.renderer != nil && shouldRenderMarkdown(body) {
		rendered, err := m.renderer.Render(body)
		if err == nil {
			return strings.TrimSpace(rendered)
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("252")).MaxWidth(maxInt(m.viewport.Width-8, 20)).Render(body)
}

func (m model) renderCommandPalette() string {
	commands := m.filteredPaletteCommands()
	if len(commands) == 0 {
		commands = []paletteCommand{{Section: "Commands", Title: "No matches", Shortcut: "esc", Summary: "Clear the search or close the palette.", Action: "noop"}}
	}
	if m.paletteIndex >= len(commands) {
		m.paletteIndex = len(commands) - 1
	}
	if m.paletteIndex < 0 {
		m.paletteIndex = 0
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render("Commands") + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(strings.Repeat(" ", maxInt(2, 24-len("Commands")))+"esc"),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("Search"),
		paletteSearchStyle().Render(valueOrFallback(m.paletteQuery, "type to filter commands")),
	}
	section := ""
	for idx, command := range commands {
		if command.Section != section {
			section = command.Section
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("177")).Render(section))
		}
		item := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(maxInt(m.viewport.Width-24, 12)).Render(command.Title),
			lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(command.Shortcut),
		)
		if idx == m.paletteIndex {
			item = lipgloss.NewStyle().Foreground(lipgloss.Color("216")).Bold(true).BorderLeft(true).BorderForeground(lipgloss.Color("216")).PaddingLeft(1).Render(item)
		}
		lines = append(lines, item)
	}
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).Width(minInt(maxInt(m.viewport.Width-6, 36), 68)).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.viewport.Width, m.viewport.Height, lipgloss.Center, lipgloss.Top, box)
}

func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	commands := m.filteredPaletteCommands()
	if len(commands) == 0 {
		commands = []paletteCommand{}
	}

	switch msg.String() {
	case "esc", "ctrl+p":
		m.paletteOpen = false
		m.refreshViewport()
		return m, nil
	case "up", "shift+tab", "ctrl+k":
		if len(commands) > 0 {
			m.paletteIndex = (m.paletteIndex - 1 + len(commands)) % len(commands)
		}
		return m, nil
	case "down", "ctrl+j":
		if len(commands) > 0 {
			m.paletteIndex = (m.paletteIndex + 1) % len(commands)
		}
		return m, nil
	case "enter":
		if len(commands) > 0 {
			m.runPaletteAction(commands[m.paletteIndex].Action)
		}
		m.paletteOpen = false
		m.refreshViewport()
		return m, nil
	case "backspace":
		if len(m.paletteQuery) > 0 {
			m.paletteQuery = m.paletteQuery[:len(m.paletteQuery)-1]
			m.paletteIndex = 0
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.paletteQuery += msg.String()
			m.paletteIndex = 0
		}
		return m, nil
	}
}

func (m *model) runPaletteAction(action string) {
	switch action {
	case "toggle_logs":
		m.showToolLogs = !m.showToolLogs
	case "clear":
		m.entries = nil
		m.appendEntry(entrySystem, "Session", "Transcript cleared from the command palette.", false)
	}
}

func (m model) filteredPaletteCommands() []paletteCommand {
	commands := paletteCommands()
	query := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	if query == "" {
		return commands
	}
	filtered := make([]paletteCommand, 0, len(commands))
	for _, command := range commands {
		haystack := strings.ToLower(command.Section + " " + command.Title + " " + command.Summary + " " + command.Shortcut)
		if strings.Contains(haystack, query) {
			filtered = append(filtered, command)
		}
	}
	return filtered
}

func paletteCommands() []paletteCommand {
	return []paletteCommand{
		{Section: "Suggested", Title: "Toggle tool logs", Shortcut: "ctrl+t", Summary: "Hide or expand background tool output.", Action: "toggle_logs"},
		{Section: "Session", Title: "Clear transcript", Shortcut: "ctrl+l", Summary: "Clear the visible transcript while keeping session context.", Action: "clear"},
	}
}

func (m model) renderHeader() string {
	left := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render("Navi Orchestrator")
	return renderBar(m.width, left, "")
}

func (m model) renderInput() string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("│")
	content := lipgloss.NewStyle().Width(maxInt(m.width-10, 12)).Render(renderInputValue(m.textarea.Value(), m.textarea.Placeholder, m.textarea.Line(), m.textarea.LineInfo().CharOffset))
	lines := []string{lipgloss.JoinHorizontal(lipgloss.Top, accent+" ", content)}
	if m.mention.Active && len(m.mention.Suggestions) > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("177")).Render("@ matches: "+strings.Join(m.mention.Suggestions, "   ")))
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(maxInt(m.width-2, 18))
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) renderStatusBar() string {
	left := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render("Orchestrator")
	rightParts := []string{"enter send", "ctrl+j newline", "ctrl+p commands", "ctrl+t logs"}
	if m.pending {
		rightParts = append(rightParts, m.spinner.View()+" waiting")
	}
	if !m.compactedAt.IsZero() && time.Since(m.compactedAt) < 12*time.Second {
		rightParts = append(rightParts, "summary refreshed")
	}
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(strings.Join(rightParts, "   "))
	return renderBar(m.width, lipgloss.NewStyle().Padding(0, 1).Render(left), lipgloss.NewStyle().Padding(0, 1).Render(right))
}

func (m model) renderMainPane() string {
	if m.paletteOpen {
		return m.renderCommandPalette()
	}
	return m.viewport.View()
}

func (m *model) appendEntry(kind entryKind, title string, body string, collapsible bool) {
	m.entries = append(m.entries, transcriptEntry{Kind: kind, Title: title, Body: body, Collapsible: collapsible, Timestamp: time.Now()})
	telemetry.Logger().Info("tui_entry", "kind", fmt.Sprintf("%d", kind), "title", title, "chars", len(body))
}

func (m *model) appendHistory(prompt string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == prompt {
		m.historyIndex = len(m.history)
		return
	}
	m.history = append(m.history, prompt)
	if len(m.history) > historyLimit {
		m.history = m.history[len(m.history)-historyLimit:]
	}
	m.historyIndex = len(m.history)
	saveHistory(m.historyPath, m.history)
}

func (m *model) historyBack() {
	if len(m.history) == 0 || m.historyIndex <= 0 {
		return
	}
	m.historyIndex--
	m.textarea.SetValue(m.history[m.historyIndex])
	m.syncTextareaHeight()
	if m.textarea.LineCount() > 0 {
		m.textarea.CursorEnd()
	}
}

func (m *model) historyForward() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex >= len(m.history)-1 {
		m.historyIndex = len(m.history)
		m.textarea.Reset()
		m.syncTextareaHeight()
		return
	}
	m.historyIndex++
	m.textarea.SetValue(m.history[m.historyIndex])
	m.syncTextareaHeight()
	m.textarea.CursorEnd()
}

func (m *model) refreshMention() {
	lineIdx := m.textarea.Line()
	lines := strings.Split(m.textarea.Value(), "\n")
	if lineIdx < 0 || lineIdx >= len(lines) {
		m.mention = mentionState{}
		return
	}
	_, _, query, ok := activeMention(lines[lineIdx], m.textarea.LineInfo().CharOffset)
	if !ok {
		m.mention = mentionState{}
		return
	}
	matches := matchWorkspaceFiles(m.workspaceFiles, query, fileSuggestionLimit)
	m.mention = mentionState{Active: len(matches) > 0, Query: query, Suggestions: matches}
}

func (m *model) completeMention() bool {
	if !m.mention.Active || len(m.mention.Suggestions) == 0 {
		return false
	}
	lineIdx := m.textarea.Line()
	updated := replaceMentionAtCursor(m.textarea.Value(), lineIdx, m.textarea.LineInfo().CharOffset, m.mention.Suggestions[0])
	m.textarea.SetValue(updated)
	m.syncTextareaHeight()
	m.textarea.CursorEnd()
	m.refreshMention()
	return true
}

func (m *model) syncTextareaHeight() {
	height := targetTextareaHeight(m.textarea.Value())
	m.textarea.SetHeight(height)
}

func targetTextareaHeight(value string) int {
	height := explicitLineCount(value)
	if height < defaultInputHeight {
		return defaultInputHeight
	}
	if height > maxInputHeight {
		return maxInputHeight
	}
	return height
}

func explicitLineCount(value string) int {
	return strings.Count(value, "\n") + 1
}

func renderInputValue(value string, placeholder string, lineIdx int, charOffset int) string {
	cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
	if value == "" {
		return cursor + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(placeholder)
	}

	lines := strings.Split(value, "\n")
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}
	runes := []rune(lines[lineIdx])
	if charOffset < 0 {
		charOffset = 0
	}
	if charOffset > len(runes) {
		charOffset = len(runes)
	}
	lines[lineIdx] = string(runes[:charOffset]) + cursor + string(runes[charOffset:])
	return strings.Join(lines, "\n")
}

func buildPrompt(prompt string, summary string, turns []conversationTurn) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	if summary == "" && len(turns) == 0 {
		return prompt
	}

	lines := []string{
		"Session context for continuity:",
	}
	if strings.TrimSpace(summary) != "" {
		lines = append(lines, "Conversation summary:", strings.TrimSpace(summary), "")
	}
	if len(turns) > 0 {
		lines = append(lines, "Recent conversation:")
		start := len(turns) - 4
		if start < 0 {
			start = 0
		}
		for _, turn := range turns[start:] {
			lines = append(lines,
				"User: "+strings.TrimSpace(turn.User),
				"Assistant: "+strings.TrimSpace(turn.Assistant),
			)
		}
		lines = append(lines, "")
	}
	lines = append(lines, "Current request:", prompt)
	return strings.Join(lines, "\n")
}

func compactTurns(summary string, turns []conversationTurn, contextLimit int) (string, []conversationTurn, string) {
	if contextLimit <= 0 {
		contextLimit = defaultContextLimit
	}
	threshold := int(float64(contextLimit) * compactionThresholdPct)
	if estimateTokens(summary)+turnTokenCount(turns) <= threshold || len(turns) <= 4 {
		return summary, turns, ""
	}

	keep := 4
	if len(turns) < keep {
		keep = len(turns)
	}
	older := turns[:len(turns)-keep]
	retained := turns[len(turns)-keep:]

	summaryLines := make([]string, 0, len(older)+1)
	if strings.TrimSpace(summary) != "" {
		summaryLines = append(summaryLines, strings.TrimSpace(summary))
	}
	for _, turn := range older {
		summaryLines = append(summaryLines, summarizeTurn(turn))
	}
	newSummary := strings.Join(summaryLines, "\n")
	if len([]rune(newSummary)) > 4000 {
		newSummary = string([]rune(newSummary)[:4000]) + "..."
	}
	notice := fmt.Sprintf("Conversation compacted automatically to stay within the context window. Retained %d recent turns.", len(retained))
	return newSummary, retained, notice
}

func summarizeTurn(turn conversationTurn) string {
	user := compressWhitespace(turn.User)
	assistant := compressWhitespace(turn.Assistant)
	if len([]rune(user)) > 120 {
		user = string([]rune(user)[:120]) + "..."
	}
	if len([]rune(assistant)) > 180 {
		assistant = string([]rune(assistant)[:180]) + "..."
	}
	return "- User asked: " + user + " | Assistant replied: " + assistant
}

func turnTokenCount(turns []conversationTurn) int {
	total := 0
	for _, turn := range turns {
		total += estimateTokens(turn.User)
		total += estimateTokens(turn.Assistant)
	}
	return total
}

func estimateTokens(text string) int {
	runes := len([]rune(strings.TrimSpace(text)))
	if runes == 0 {
		return 0
	}
	tokens := runes / 4
	if runes%4 != 0 {
		tokens++
	}
	return maxInt(tokens, 1)
}

func estimateCostDisplay(model string, inputTokens int, outputTokens int) string {
	prices, ok := pricingForModel(model)
	if !ok {
		return "n/a"
	}
	cost := (float64(inputTokens)/1_000_000.0)*prices[0] + (float64(outputTokens)/1_000_000.0)*prices[1]
	if cost == 0 {
		return "$0.0000"
	}
	return fmt.Sprintf("$%.4f", cost)
}

func pricingForModel(model string) ([2]float64, bool) {
	name := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(name, "gpt-4o-mini"):
		return [2]float64{0.15, 0.60}, true
	case strings.Contains(name, "gpt-4.1-mini"):
		return [2]float64{0.40, 1.60}, true
	case strings.Contains(name, "gpt-4.1"):
		return [2]float64{2.00, 8.00}, true
	case strings.Contains(name, "gpt-4o"):
		return [2]float64{2.50, 10.00}, true
	default:
		return [2]float64{}, false
	}
}

func defaultHistoryPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "repl_history.json"), nil
}

func loadHistory(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var store historyStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil
	}
	if len(store.Prompts) > historyLimit {
		store.Prompts = store.Prompts[len(store.Prompts)-historyLimit:]
	}
	return store.Prompts
}

func saveHistory(path string, prompts []string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(historyStore{Prompts: prompts}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func discoverWorkspaceFiles(root string, limit int) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	files := make([]string, 0, minInt(limit, 256))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, rel)
		if len(files) >= limit {
			return errStopWalk
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func shouldSkipDir(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	switch base {
	case ".git", "node_modules", "coverage", ".idea", ".vscode":
		return true
	default:
		return false
	}
}

func matchWorkspaceFiles(files []string, query string, limit int) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		if len(files) > limit {
			return append([]string(nil), files[:limit]...)
		}
		return append([]string(nil), files...)
	}
	starts := make([]string, 0, limit)
	contains := make([]string, 0, limit)
	for _, file := range files {
		lower := strings.ToLower(file)
		if strings.HasPrefix(lower, query) {
			starts = append(starts, file)
			if len(starts) >= limit {
				break
			}
			continue
		}
		if strings.Contains(lower, query) && len(contains) < limit {
			contains = append(contains, file)
		}
	}
	combined := append(starts, contains...)
	if len(combined) > limit {
		combined = combined[:limit]
	}
	return combined
}

func activeMention(line string, cursorOffset int) (int, int, string, bool) {
	runes := []rune(line)
	if cursorOffset < 0 {
		cursorOffset = 0
	}
	if cursorOffset > len(runes) {
		cursorOffset = len(runes)
	}
	prefix := string(runes[:cursorOffset])
	start := strings.LastIndexAny(prefix, " \t") + 1
	token := string([]rune(prefix)[start:])
	if !strings.HasPrefix(token, "@") {
		return 0, 0, "", false
	}
	query := strings.TrimPrefix(token, "@")
	return start, cursorOffset, query, true
}

func replaceMentionAtCursor(text string, lineIdx int, cursorOffset int, replacement string) string {
	lines := strings.Split(text, "\n")
	if lineIdx < 0 || lineIdx >= len(lines) {
		return text
	}
	start, end, _, ok := activeMention(lines[lineIdx], cursorOffset)
	if !ok {
		return text
	}
	runes := []rune(lines[lineIdx])
	updated := string(runes[:start]) + "@" + replacement + string(runes[end:])
	lines[lineIdx] = updated
	return strings.Join(lines, "\n")
}

func shouldRenderMarkdown(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "```") || strings.Contains(trimmed, "|---") || strings.Contains(trimmed, "# ") || strings.Contains(trimmed, "## ") || strings.Contains(trimmed, "**") {
		return true
	}
	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "1. ") {
			return true
		}
	}
	return false
}

func extractStandaloneDiff(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "```diff") && strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```diff")
		trimmed = strings.TrimSuffix(trimmed, "```")
		return strings.TrimSpace(trimmed), true
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 {
		return "", false
	}
	diffish := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff ") {
			diffish++
		}
	}
	return trimmed, diffish >= maxInt(2, len(lines)/3)
}

func renderDiff(diff string) string {
	lines := strings.Split(strings.TrimSpace(diff), "\n")
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			styled = append(styled, lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			styled = append(styled, lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Render(line))
		case strings.HasPrefix(line, "@@"):
			styled = append(styled, lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(line))
		default:
			styled = append(styled, lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(line))
		}
	}
	return strings.Join(styled, "\n")
}

func looksLikeJSON(body string) bool {
	return json.Valid([]byte(body))
}

func shortPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return "."
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 3 {
		return path
	}
	return strings.Join(parts[len(parts)-3:], "/")
}

func renderMeter(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

func valueOrFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func compressWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func renderEntryHeader(entry transcriptEntry) string {
	accent := entryAccentStyle(entry.Kind).Render(entry.Title)
	timeText := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(entry.Timestamp.Format("15:04:05"))
	return lipgloss.NewStyle().MaxWidth(64).Render(accent) + "  " + timeText
}

func entryAccentStyle(kind entryKind) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(entryColor(kind)).Bold(kind == entryAssistant || kind == entryUser || kind == entryError)
}

func paletteSearchStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("252")).BorderLeft(true).BorderForeground(lipgloss.Color("240")).PaddingLeft(1)
}

func entryColor(kind entryKind) lipgloss.Color {
	switch kind {
	case entryThinking:
		return lipgloss.Color("180")
	case entryTool:
		return lipgloss.Color("177")
	case entryAssistant:
		return lipgloss.Color("216")
	case entryError:
		return lipgloss.Color("203")
	case entryUser:
		return lipgloss.Color("81")
	default:
		return lipgloss.Color("252")
	}
}

func truncateForTitle(value string) string {
	value = compressWhitespace(value)
	runes := []rune(value)
	if len(runes) <= 72 {
		return value
	}
	return string(runes[:72]) + "..."
}

func stylesForEntry(kind entryKind) (lipgloss.Style, lipgloss.Style) {
	switch kind {
	case entryUser:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Padding(0, 1),
			lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("62")).PaddingLeft(1)
	case entryThinking:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("94")).Padding(0, 1),
			lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("94")).PaddingLeft(1)
	case entryTool:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("151")).Padding(0, 1),
			lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("151")).PaddingLeft(1)
	case entryAssistant:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("24")).Padding(0, 1),
			lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("24")).PaddingLeft(1)
	case entryError:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("160")).Padding(0, 1),
			lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("160")).PaddingLeft(1)
	default:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Padding(0, 1),
			lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("238")).PaddingLeft(1)
	}
}

func infoBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("250")).BorderLeft(true).BorderForeground(lipgloss.Color("238")).PaddingLeft(1)
}

func renderBar(totalWidth int, left string, right string) string {
	if totalWidth <= 0 {
		totalWidth = 20
	}
	innerWidth := maxInt(totalWidth-2, 10)
	gapWidth := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gapWidth < 1 {
		gapWidth = 1
	}
	return lipgloss.NewStyle().
		Padding(0, 1).
		Width(innerWidth).
		Render(left + strings.Repeat(" ", gapWidth) + right)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
