package main

import (
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"github.com/bear/charmkit"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time
type focus int

const (
	focusLibrary focus = iota
	focusQueue
)

type track struct {
	path     string
	title    string
	artist   string
	duration time.Duration
	staged   bool
	playing  bool
}

func (t track) Title() string       { return t.title }
func (t track) Description() string { return t.artist }
func (t track) FilterValue() string { return t.title + " " + t.artist }

// trackDelegate renders each entry as a single compact line.
type trackDelegate struct {
	showStaged bool
	focused    bool
}

func (d trackDelegate) Height() int  { return 1 }
func (d trackDelegate) Spacing() int { return 0 }
func (d trackDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d trackDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	t, ok := item.(track)
	if !ok {
		return
	}

	selected := d.focused && index == m.Index()

	numS, titleS, artistS, durS := mutedStyle, titleStyle, artistStyle, mutedStyle
	if selected {
		numS = mutedStyle.Copy().Foreground(accent)
		titleS = titleStyle.Copy().Foreground(accent)
		artistS = artistStyle.Copy().Foreground(accent)
		durS = mutedStyle.Copy().Foreground(accent)
	}

	prefix := "  "
	if t.playing {
		prefix = accentStyle.Render("▶ ")
	} else if d.showStaged && t.staged {
		prefix = accentStyle.Render("● ")
	}

	dur := durS.Render(fmtDur(t.duration))
	durW := lipgloss.Width(dur)

	width := m.Width()
	avail := width - 2 - 4 - durW - 4 // prefix + num + two gaps
	if avail < 4 {
		avail = 4
	}
	titleW := avail / 2
	artistW := avail - titleW

	num := numS.Width(4).Render(fmt.Sprintf("%02d", index+1))
	title := titleS.Width(titleW).Render(truncate(t.title, titleW))
	artist := artistS.Width(artistW).Render(truncate(t.artist, artistW))

	fmt.Fprint(w, prefix+num+title+"  "+artist+"  "+dur)
}

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Tab      key.Binding
	Stage    key.Binding
	Enter    key.Binding
	Prev     key.Binding
	Next     key.Binding
	SeekFwd  key.Binding
	SeekBack key.Binding
	VolUp    key.Binding
	VolDown  key.Binding
	Add      key.Binding
	Unstage  key.Binding
	Shuffle  key.Binding
	Repeat   key.Binding
	Confirm  key.Binding
	Cancel   key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Stage, k.Enter, k.Tab, k.Next, k.Prev, k.SeekFwd, k.SeekBack, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Stage, k.Enter, k.Tab, k.Next, k.Prev},
		{k.SeekFwd, k.SeekBack, k.VolUp, k.VolDown},
		{k.Add, k.Unstage, k.Shuffle, k.Repeat, k.Help, k.Quit},
	}
}

var ck = charmkit.NewKeys()

var keys = keyMap{
	Up:       ck.Up,
	Down:     ck.Down,
	Tab:      ck.Tab,
	Stage:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "stage")),
	Enter:    ck.Enter,
	Prev:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev")),
	Next:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
	SeekFwd:  key.NewBinding(key.WithKeys(">", ".", "right", "l"), key.WithHelp(">", "seek +10s")),
	SeekBack: key.NewBinding(key.WithKeys("<", ",", "left", "h"), key.WithHelp("<", "seek -10s")),
	VolUp:    key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+", "vol up")),
	VolDown:  key.NewBinding(key.WithKeys("-", "_"), key.WithHelp("-", "vol down")),
	Add:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add files")),
	Unstage:  key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "unstage")),
	Shuffle:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shuffle")),
	Repeat:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeat")),
	Confirm:  ck.Enter,
	Cancel:   ck.Back,
	Help:     ck.Help,
	Quit:     ck.Quit,
}

type Model struct {
	player        *Player
	library       []track
	queue         []track
	libraryList   list.Model
	queueList     list.Model
	progress      progress.Model
	input         textinput.Model
	adding        bool
	focus         focus
	current       int // index into queue
	width         int
	height        int
	paneW         int
	paneH         int
	wide          bool
	showHeader    bool
	compactHeader bool
	shuffle       bool
	repeat        bool
	showHelp      bool
	lastTick      int64
	err           string
}

func NewModel(library []track, p *Player) Model {
	libItems := make([]list.Item, len(library))
	for i, t := range library {
		libItems[i] = t
	}

	libList := list.New(libItems, trackDelegate{showStaged: true, focused: true}, 0, 0)
	libList.SetShowTitle(false)
	libList.SetShowStatusBar(false)
	libList.SetShowHelp(false)
	libList.SetShowPagination(false)
	libList.SetFilteringEnabled(false)

	queueList := list.New([]list.Item{}, trackDelegate{showStaged: false, focused: false}, 0, 0)
	queueList.SetShowTitle(false)
	queueList.SetShowStatusBar(false)
	queueList.SetShowHelp(false)
	queueList.SetShowPagination(false)
	queueList.SetFilteringEnabled(false)

	prog := progress.New(progress.WithScaledGradient("2", "7"))
	prog.ShowPercentage = false

	in := textinput.New()
	in.Placeholder = "path to .mp3 or directory"
	in.Prompt = accentStyle.Render("add » ")
	in.CharLimit = 512

	return Model{
		player:      p,
		library:     library,
		libraryList: libList,
		queueList:   queueList,
		progress:    prog,
		input:       in,
		focus:       focusLibrary,
		current:     -1,
	}
}

func (m *Model) focusedList() *list.Model {
	if m.focus == focusQueue {
		return &m.queueList
	}
	return &m.libraryList
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	m.libraryList.SetDelegate(trackDelegate{showStaged: true, focused: f == focusLibrary})
	m.queueList.SetDelegate(trackDelegate{showStaged: false, focused: f == focusQueue})
}

// stage marks the library track at index i and appends it to the queue.
func (m *Model) stage(i int) {
	if i < 0 || i >= len(m.library) || m.library[i].staged {
		return
	}
	m.library[i].staged = true
	m.queue = append(m.queue, m.library[i])
	m.queueList.InsertItem(len(m.queue)-1, m.library[i])
	m.libraryList.SetItem(i, m.library[i])
	m.syncPlaying()
}

// unstagePath removes a track from the queue by file path.
func (m *Model) unstagePath(path string) {
	for qi := range m.queue {
		if m.queue[qi].path == path {
			m.unstageAt(qi)
			return
		}
	}
}

// unstageAt removes the queue track at index qi.
func (m *Model) unstageAt(qi int) {
	if qi < 0 || qi >= len(m.queue) {
		return
	}
	path := m.queue[qi].path
	for i := range m.library {
		if m.library[i].path == path {
			m.library[i].staged = false
			m.libraryList.SetItem(i, m.library[i])
			break
		}
	}
	m.removeFromQueue(qi)
}

func (m *Model) removeFromQueue(qi int) {
	wasPlaying := qi == m.current
	m.queue = append(m.queue[:qi], m.queue[qi+1:]...)
	m.queueList.RemoveItem(qi)
	switch {
	case qi < m.current:
		m.current--
	case qi == m.current:
		if wasPlaying {
			m.player.Stop()
		}
		if len(m.queue) == 0 {
			m.current = -1
		} else if m.current >= len(m.queue) {
			m.current = len(m.queue) - 1
		}
		if wasPlaying && m.current >= 0 {
			m.play(m.current)
		}
	}
	m.syncPlaying()
}

// toggleStage stages or unstages the selection based on the focused pane.
func (m *Model) toggleStage() {
	if m.focus == focusLibrary {
		i := m.libraryList.Index()
		if i < 0 || i >= len(m.library) {
			return
		}
		if m.library[i].staged {
			m.unstagePath(m.library[i].path)
		} else {
			m.stage(i)
		}
		return
	}
	m.unstageAt(m.queueList.Index())
}

// enter plays the selection (from the library it just stages, without
// interrupting playback; from the queue it starts or pauses the selected song).
func (m *Model) enter() {
	if m.focus == focusQueue {
		qi := m.queueList.Index()
		if qi == m.current && m.current >= 0 {
			m.player.Toggle()
			return
		}
		m.play(qi)
		return
	}

	i := m.libraryList.Index()
	if i < 0 || i >= len(m.library) {
		return
	}
	if m.library[i].playing {
		m.player.Toggle()
		return
	}
	m.stage(i)
}

func (m *Model) play(i int) {
	if i < 0 || i >= len(m.queue) {
		return
	}
	m.current = i
	m.queueList.Select(i)
	if err := m.player.Load(m.queue[i].path); err != nil {
		m.err = err.Error()
		return
	}
	m.err = ""
	m.queue[i].duration = m.player.Length()
	m.player.Play()
	m.syncPlaying()
}

func (m *Model) next() {
	if len(m.queue) == 0 {
		return
	}
	if m.current < 0 {
		m.play(0)
		return
	}
	if m.shuffle && len(m.queue) > 1 {
		n := rand.Intn(len(m.queue))
		for n == m.current {
			n = rand.Intn(len(m.queue))
		}
		m.play(n)
		return
	}
	m.play((m.current + 1) % len(m.queue))
}

func (m *Model) prev() {
	if len(m.queue) == 0 {
		return
	}
	if m.current < 0 {
		m.play(0)
		return
	}
	m.play((m.current - 1 + len(m.queue)) % len(m.queue))
}

func (m *Model) isCurrentPath(path string) bool {
	return m.current >= 0 && m.current < len(m.queue) && m.queue[m.current].path == path
}

func (m *Model) syncPlaying() {
	for i := range m.library {
		m.library[i].playing = m.isCurrentPath(m.library[i].path)
		m.libraryList.SetItem(i, m.library[i])
	}
	for i := range m.queue {
		m.queue[i].playing = i == m.current
		m.queueList.SetItem(i, m.queue[i])
	}
}

// add appends any .mp3 files found at path to the library (unstaged).
func (m *Model) add(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, t := range gatherTracks([]string{path}) {
		m.library = append(m.library, t)
		m.libraryList.InsertItem(len(m.library)-1, t)
	}
}

func (m Model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/4, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		m.lastTick = int64(time.Time(msg).UnixMilli())
		if m.player.IsPlaying() && len(m.queue) > 0 {
			if pos, ln := m.player.Position(), m.player.Length(); ln > 0 && pos >= ln {
				if m.repeat {
					m.player.Seek(0)
				} else {
					m.next()
				}
			}
		}
		return m, tick()

	case tea.KeyMsg:
		if m.adding {
			switch {
			case key.Matches(msg, keys.Confirm):
				m.add(m.input.Value())
				m.adding = false
				m.input.SetValue("")
				m.input.Blur()
				if m.width > 0 {
					m.resize()
				}
			case key.Matches(msg, keys.Cancel):
				m.adding = false
				m.input.SetValue("")
				m.input.Blur()
				if m.width > 0 {
					m.resize()
				}
			default:
				var icmd tea.Cmd
				m.input, icmd = m.input.Update(msg)
				return m, icmd
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Quit):
			m.player.Stop()
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp
			if m.width > 0 {
				m.resize()
			}
			return m, nil
		case key.Matches(msg, keys.Tab):
			if m.focus == focusLibrary {
				m.setFocus(focusQueue)
			} else {
				m.setFocus(focusLibrary)
			}
			return m, nil
		case key.Matches(msg, keys.Stage):
			m.toggleStage()
			return m, nil
		case key.Matches(msg, keys.Enter):
			m.enter()
			return m, nil
		case key.Matches(msg, keys.Add):
			m.adding = true
			m.input.Focus()
			if m.width > 0 {
				m.resize()
			}
			return m, nil
		case key.Matches(msg, keys.Unstage):
			if m.focus == focusQueue {
				m.unstageAt(m.queueList.Index())
			} else if i := m.libraryList.Index(); i >= 0 && i < len(m.library) {
				m.unstagePath(m.library[i].path)
			}
			return m, nil
		case key.Matches(msg, keys.Next):
			m.next()
			return m, nil
		case key.Matches(msg, keys.Prev):
			m.prev()
			return m, nil
		case key.Matches(msg, keys.SeekFwd):
			m.player.Seek(10 * time.Second)
			return m, nil
		case key.Matches(msg, keys.SeekBack):
			m.player.Seek(-10 * time.Second)
			return m, nil
		case key.Matches(msg, keys.VolUp):
			m.player.VolumeUp()
			return m, nil
		case key.Matches(msg, keys.VolDown):
			m.player.VolumeDown()
			return m, nil
		case key.Matches(msg, keys.Shuffle):
			m.shuffle = !m.shuffle
			return m, nil
		case key.Matches(msg, keys.Repeat):
			m.repeat = !m.repeat
			return m, nil
		case key.Matches(msg, keys.Up):
			m.focusedList().CursorUp()
			return m, nil
		case key.Matches(msg, keys.Down):
			m.focusedList().CursorDown()
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focus == focusQueue {
		m.queueList, cmd = m.queueList.Update(msg)
	} else {
		m.libraryList, cmd = m.libraryList.Update(msg)
	}
	return m, cmd
}

func (m *Model) resize() {
	headerFull := 4    // border + title + progress
	headerCompact := 3 // border + title only

	m.wide = m.width >= 72
	if m.wide {
		m.paneW = m.width / 2
	} else {
		m.paneW = m.width
	}

	// The bottom bar always takes at least one row (short help or the add-path
	// prompt) and grows to the full table when help is expanded.
	footerH := m.footerH()

	// Panes are the flexible region. As the terminal shrinks we collapse the
	// header to a single line, then drop it entirely, so "now playing" stays
	// visible as long as possible. The bottom bar is always shown.
	m.showHeader = true
	m.compactHeader = false
	m.paneH = m.height - headerFull - footerH
	if m.paneH < 3 {
		m.compactHeader = true
		m.paneH = m.height - headerCompact - footerH
	}
	if m.paneH < 3 {
		m.showHeader = false
		m.paneH = m.height - footerH
	}
	if m.paneH < 3 {
		m.paneH = 3
	}

	lw := m.paneW - 4 // border + horizontal padding
	if lw < 2 {
		lw = 2
	}
	lh := m.paneH - 3 // title + borders
	if lh < 0 {
		lh = 0
	}
	m.libraryList.SetSize(lw, lh)
	m.queueList.SetSize(lw, lh)
}

func (m Model) View() string {
	if m.width == 0 {
		m.resize()
	}

	var parts []string
	if m.showHeader {
		parts = append(parts, box.Width(m.width-2).Render(m.headerContent()))
	}
	parts = append(parts, m.bodyView())
	parts = append(parts, m.footer())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// footerH returns how many rows the bottom bar needs: one line for the short
// help or the add-path prompt, or the full binding table when help is open.
func (m Model) footerH() int {
	if m.adding {
		return 1
	}
	if m.showHelp {
		return len(strings.Split(charmkit.Help(m.width, keys.ShortHelp(), keys.FullHelp(), true), "\n"))
	}
	return 1
}

// footer renders the bottom bar: the add-path prompt while entering a path,
// otherwise the (short or full) keybinding help shared via charmkit.
func (m Model) footer() string {
	if m.adding {
		return m.input.View()
	}
	return charmkit.Help(m.width, keys.ShortHelp(), keys.FullHelp(), m.showHelp)
}

func (m Model) pane(title, items string, focused bool, w, h int) string {
	titleS := titleStyle
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if focused {
		titleS = titleStyle.Copy().Foreground(accent).Bold(true)
		style = style.BorderForeground(accent)
	} else {
		style = style.BorderForeground(subtle)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, titleS.Render(title), items)
	// Width/Height size the content area; the rounded border adds two columns
	// and two rows on top, so back it out to hit w×h exactly.
	return style.Width(w - 2).Height(h - 2).Render(content)
}

func (m Model) bodyView() string {
	lib := m.pane("Library", m.libraryList.View(), m.focus == focusLibrary, m.paneW, m.paneH)
	que := m.pane("Playlist", m.queueList.View(), m.focus == focusQueue, m.paneW, m.paneH)

	if m.wide {
		return lipgloss.JoinHorizontal(lipgloss.Top, lib, que)
	}
	if m.focus == focusQueue {
		return que
	}
	return lib
}

func (m Model) headerContent() string {
	if len(m.queue) == 0 || m.current < 0 {
		if m.compactHeader {
			return mutedStyle.Render(truncate("nothing playing", m.width-4))
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			mutedStyle.Render(truncate("nothing playing", m.width-4)),
			mutedStyle.Render(truncate("space to stage · enter to play", m.width-4)),
		)
	}
	if m.compactHeader {
		return m.nowPlayingLine(m.width - 4)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.nowPlayingLine(m.width-4),
		m.progressLine(m.width-4),
	)
}

func (m Model) nowPlayingLine(w int) string {
	t := m.queue[m.current]
	state := "❚❚"
	if m.player.IsPlaying() {
		state = "▶"
	}

	shuffle := mutedStyle.Render("⇄")
	if m.shuffle {
		shuffle = accentStyle.Render("⇄")
	}
	repeat := mutedStyle.Render("↻")
	if m.repeat {
		repeat = accentStyle.Render("↻")
	}
	right := mutedStyle.Render(fmtDur(t.duration)) + "  " + shuffle + " " + repeat

	stateStr := state + " "
	info := t.title
	if t.artist != "" {
		info += " · " + t.artist
	}
	avail := w - lipgloss.Width(right) - 1
	if avail < 4 {
		avail = 4
	}
	info = truncate(info, avail-lipgloss.Width(stateStr))

	var left string
	if i := strings.Index(info, " · "); i > 0 {
		left = accentStyle.Render(stateStr) + titleStyle.Render(info[:i]) + artistStyle.Render(info[i:])
	} else {
		left = accentStyle.Render(stateStr) + titleStyle.Render(info)
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) progressLine(w int) string {
	pos := m.player.Position()
	ln := m.player.Length()
	frac := 0.0
	if ln > 0 {
		frac = float64(pos) / float64(ln)
	}
	if frac > 1 {
		frac = 1
	}
	m.progress.Width = w - 15
	bar := m.progress.ViewAs(frac)
	times := mutedStyle.Render(lipgloss.NewStyle().Width(13).Align(lipgloss.Right).Render(
		fmt.Sprintf("%s / %s", fmtDur(pos), fmtDur(ln)),
	))
	return lipgloss.JoinHorizontal(lipgloss.Center, bar, "  "+times)
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	if w == 1 {
		return "…"
	}
	// Accumulate runes until the next one would push past w-1, reserving the
	// last column for the ellipsis. Wide (fullwidth) runes count as two, so we
	// must measure display width rather than count runes.
	target := w - 1
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > target {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String() + "…"
}
