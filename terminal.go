package main

import (
	"image/color"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Theme colors for the terminal.
var (
	ColorBG        = color.RGBA{18, 20, 26, 255}    // Deep dark slate background
	ColorFG        = color.RGBA{220, 225, 235, 255} // Crisp light gray default text
	ColorPrompt    = color.RGBA{80, 200, 120, 255}  // Emerald green prompt
	ColorCmd       = color.RGBA{255, 255, 255, 255} // Bright white typed input
	ColorInfo      = color.RGBA{100, 180, 255, 255} // Sky blue info text
	ColorWarn      = color.RGBA{255, 200, 80, 255}  // Amber warning text
	ColorError     = color.RGBA{255, 95, 95, 255}   // Coral red error text
	ColorMuted     = color.RGBA{110, 120, 140, 255} // Muted gray comments/timestamps
	ColorCursor    = color.RGBA{80, 200, 120, 220}  // Block/bar cursor color
	ColorSelection = color.RGBA{55, 90, 145, 255}   // Highlighted selection background
)

// Span represents a run of text with a single foreground color.
type Span struct {
	Text string
	FG   color.RGBA
}

// Line represents a single logical line in the terminal scrollback buffer.
type Line struct {
	Spans []Span
}

// StyledCell is a single character cell after wrapping a Line to the current column width.
type StyledCell struct {
	Char rune
	FG   color.RGBA
}

// GridPos identifies a character cell in visual (wrapped) row/column coordinates.
type GridPos struct {
	Row int
	Col int
}

// Terminal holds the screen buffer, input line editor state, and command history.
type Terminal struct {
	mu sync.Mutex

	// Scrollback buffer of logical lines.
	Lines       []Line
	MaxLines    int
	ScrollLines int // 0 = pinned to bottom; >0 = scrolled up by N visual rows

	// Prompt and line editor state.
	PromptPrefix string
	Input        []rune
	CursorIndex  int // Index within Input (0..len(Input))

	// Command history (Up / Down arrows).
	History      []string
	HistoryIndex int
	SavedInput   string

	// Cursor blink timer (in ticks, 60 ticks = 1 sec).
	BlinkTicks int

	// Mouse text selection state (in visual row/col coordinates).
	Selecting      bool
	HasSelection   bool
	SelectStart    GridPos
	SelectEnd      GridPos
	LastVisualRows [][]StyledCell
}

// NewTerminal initializes a new Terminal with a welcome banner.
func NewTerminal() *Terminal {
	t := &Terminal{
		MaxLines:     5000,
		PromptPrefix: "tterm> ",
		HistoryIndex: -1,
	}

	t.PrintLine([]Span{
		{Text: "============================================================", FG: ColorMuted},
	})
	t.PrintLine([]Span{
		{Text: "  tterm ", FG: ColorPrompt},
		{Text: "v0.1.0 — Custom Go + Ebitengine Terminal Prototype", FG: ColorInfo},
	})
	t.PrintLine([]Span{
		{Text: "  Type ", FG: ColorFG},
		{Text: "help", FG: ColorWarn},
		{Text: " and press Enter to see available commands.", FG: ColorFG},
	})
	t.PrintLine([]Span{
		{Text: "============================================================", FG: ColorMuted},
	})
	t.PrintPlain("", ColorFG)

	return t
}

// PrintPlain appends a single-color line (supporting embedded '\n' newlines).
func (t *Terminal) PrintPlain(text string, fg color.RGBA) {
	parts := strings.Split(text, "\n")
	for _, part := range parts {
		t.PrintLine([]Span{{Text: part, FG: fg}})
	}
}

// PrintLine safely appends a styled Line to the scrollback buffer.
func (t *Terminal) PrintLine(spans []Span) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Lines = append(t.Lines, Line{Spans: spans})
	if len(t.Lines) > t.MaxLines {
		t.Lines = t.Lines[len(t.Lines)-t.MaxLines:]
	}
}

// Clear wipes the scrollback buffer.
func (t *Terminal) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Lines = nil
	t.ScrollLines = 0
	t.HasSelection = false
}

// InsertRunes inserts typed characters at the current cursor position.
func (t *Terminal) InsertRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Filter out control characters that might arrive via AppendInputChars
	clean := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r >= 32 && r != 127 {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return
	}

	left := t.Input[:t.CursorIndex]
	right := t.Input[t.CursorIndex:]
	newInput := make([]rune, 0, len(t.Input)+len(clean))
	newInput = append(newInput, left...)
	newInput = append(newInput, clean...)
	newInput = append(newInput, right...)

	t.Input = newInput
	t.CursorIndex += len(clean)
	t.BlinkTicks = 0
	t.ScrollLines = 0
}

// Backspace deletes the rune before the cursor.
func (t *Terminal) Backspace() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.CursorIndex > 0 {
		t.Input = append(t.Input[:t.CursorIndex-1], t.Input[t.CursorIndex:]...)
		t.CursorIndex--
		t.BlinkTicks = 0
	}
}

// Delete removes the rune under the cursor.
func (t *Terminal) Delete() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.CursorIndex < len(t.Input) {
		t.Input = append(t.Input[:t.CursorIndex], t.Input[t.CursorIndex+1:]...)
		t.BlinkTicks = 0
	}
}

// MoveCursor moves the caret left or right within the input line.
func (t *Terminal) MoveCursor(delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.CursorIndex += delta
	if t.CursorIndex < 0 {
		t.CursorIndex = 0
	}
	if t.CursorIndex > len(t.Input) {
		t.CursorIndex = len(t.Input)
	}
	t.BlinkTicks = 0
}

// MoveCursorWord jumps to the previous or next word boundary (Ctrl+Left / Ctrl+Right).
func (t *Terminal) MoveCursorWord(dir int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if dir < 0 {
		for t.CursorIndex > 0 && t.Input[t.CursorIndex-1] == ' ' {
			t.CursorIndex--
		}
		for t.CursorIndex > 0 && t.Input[t.CursorIndex-1] != ' ' {
			t.CursorIndex--
		}
	} else if dir > 0 {
		for t.CursorIndex < len(t.Input) && t.Input[t.CursorIndex] != ' ' {
			t.CursorIndex++
		}
		for t.CursorIndex < len(t.Input) && t.Input[t.CursorIndex] == ' ' {
			t.CursorIndex++
		}
	}
	t.BlinkTicks = 0
}

// MoveCursorHomeEnd moves cursor to start or end of the input line.
func (t *Terminal) MoveCursorHomeEnd(toEnd bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if toEnd {
		t.CursorIndex = len(t.Input)
	} else {
		t.CursorIndex = 0
	}
	t.BlinkTicks = 0
}

// NavigateHistory cycles through previous commands using Up (-1) or Down (+1).
func (t *Terminal) NavigateHistory(dir int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.History) == 0 {
		return
	}

	if t.HistoryIndex == -1 {
		if dir < 0 {
			t.SavedInput = string(t.Input)
			t.HistoryIndex = len(t.History) - 1
			t.Input = []rune(t.History[t.HistoryIndex])
			t.CursorIndex = len(t.Input)
		}
		return
	}

	t.HistoryIndex += dir
	if t.HistoryIndex < 0 {
		t.HistoryIndex = 0
	} else if t.HistoryIndex >= len(t.History) {
		t.HistoryIndex = -1
		t.Input = []rune(t.SavedInput)
		t.CursorIndex = len(t.Input)
		return
	}

	t.Input = []rune(t.History[t.HistoryIndex])
	t.CursorIndex = len(t.Input)
	t.BlinkTicks = 0
}

// SubmitLine commits the current input line to the scrollback, records it in history,
// and returns the trimmed command string for execution.
func (t *Terminal) SubmitLine() string {
	t.mu.Lock()
	raw := string(t.Input)
	t.Input = nil
	t.CursorIndex = 0
	t.HistoryIndex = -1
	t.SavedInput = ""
	t.BlinkTicks = 0
	t.ScrollLines = 0

	// Echo prompt + command into scrollback history
	t.Lines = append(t.Lines, Line{
		Spans: []Span{
			{Text: t.PromptPrefix, FG: ColorPrompt},
			{Text: raw, FG: ColorCmd},
		},
	})

	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		// Avoid duplicate consecutive entries
		if len(t.History) == 0 || t.History[len(t.History)-1] != trimmed {
			t.History = append(t.History, trimmed)
		}
	}
	t.mu.Unlock()

	return trimmed
}

// BuildVisualRows wraps all scrollback lines + the active input line to fit `cols` columns.
// It also returns the visual (row, col) of the active input cursor.
func (t *Terminal) BuildVisualRows(cols int) (rows [][]StyledCell, cursorRow, cursorCol int) {
	if cols < 10 {
		cols = 10
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	wrapLine := func(spans []Span) [][]StyledCell {
		var cells []StyledCell
		for _, s := range spans {
			for _, r := range s.Text {
				cells = append(cells, StyledCell{Char: r, FG: s.FG})
			}
		}
		if len(cells) == 0 {
			return [][]StyledCell{{}}
		}
		var wrapped [][]StyledCell
		for len(cells) > cols {
			wrapped = append(wrapped, cells[:cols])
			cells = cells[cols:]
		}
		wrapped = append(wrapped, cells)
		return wrapped
	}

	for _, line := range t.Lines {
		rows = append(rows, wrapLine(line.Spans)...)
	}

	// Active prompt + input line
	promptRunes := []rune(t.PromptPrefix)
	promptCells := make([]StyledCell, 0, len(promptRunes)+len(t.Input))
	for _, r := range promptRunes {
		promptCells = append(promptCells, StyledCell{Char: r, FG: ColorPrompt})
	}
	for _, r := range t.Input {
		promptCells = append(promptCells, StyledCell{Char: r, FG: ColorCmd})
	}

	promptStartRow := len(rows)
	totalPromptOffset := len(promptRunes) + t.CursorIndex
	cursorRow = promptStartRow + (totalPromptOffset / cols)
	cursorCol = totalPromptOffset % cols

	if len(promptCells) == 0 {
		rows = append(rows, []StyledCell{})
	} else {
		for len(promptCells) > cols {
			rows = append(rows, promptCells[:cols])
			promptCells = promptCells[cols:]
		}
		rows = append(rows, promptCells)
	}

	// If cursor is at the exact wrap edge, ensure an empty row exists for it
	if cursorRow >= len(rows) {
		rows = append(rows, []StyledCell{})
	}

	t.LastVisualRows = rows
	return rows, cursorRow, cursorCol
}

// SelectedText extracts the currently highlighted text in visual coordinates.
func (t *Terminal) SelectedText() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.HasSelection || len(t.LastVisualRows) == 0 {
		return ""
	}

	start, end := NormalizeSelection(t.SelectStart, t.SelectEnd)
	if start == end {
		return ""
	}

	var sb strings.Builder
	for r := start.Row; r <= end.Row && r < len(t.LastVisualRows); r++ {
		if r < 0 {
			continue
		}
		row := t.LastVisualRows[r]
		cStart := 0
		cEnd := len(row)
		if r == start.Row {
			cStart = start.Col
		}
		if r == end.Row {
			cEnd = end.Col + 1
		}
		if cStart < 0 {
			cStart = 0
		}
		if cEnd > len(row) {
			cEnd = len(row)
		}
		if cStart < cEnd {
			for _, cell := range row[cStart:cEnd] {
				sb.WriteRune(cell.Char)
			}
		}
		if r < end.Row {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// NormalizeSelection orders two GridPos points so start <= end.
func NormalizeSelection(a, b GridPos) (GridPos, GridPos) {
	if a.Row < b.Row || (a.Row == b.Row && a.Col <= b.Col) {
		return a, b
	}
	return b, a
}

// --- Minimal Pure-Go Windows Clipboard Helpers (no cgo required) ---

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procGetClipboard   = user32.NewProc("GetClipboardData")
	procSetClipboard   = user32.NewProc("SetClipboardData")
	procGlobalAlloc    = kernel32.NewProc("GlobalAlloc")
	procGlobalFree     = kernel32.NewProc("GlobalFree")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// GetClipboardText reads UTF-16 text from the Windows clipboard.
func GetClipboardText() string {
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboard.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)

	// Read null-terminated UTF-16 slice
	var u16 []uint16
	for i := uintptr(0); i < 65536; i += 2 {
		ch := *(*uint16)(unsafe.Pointer(ptr + i))
		if ch == 0 {
			break
		}
		u16 = append(u16, ch)
	}
	return syscall.UTF16ToString(u16)
}

// SetClipboardText writes UTF-16 text to the Windows clipboard.
func SetClipboardText(text string) {
	u16, err := syscall.UTF16FromString(text)
	if err != nil {
		return
	}
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	size := uintptr(len(u16) * 2)
	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if hMem == 0 {
		return
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return
	}

	for i, v := range u16 {
		*(*uint16)(unsafe.Pointer(ptr + uintptr(i*2))) = v
	}
	procGlobalUnlock.Call(hMem)

	if res, _, _ := procSetClipboard.Call(cfUnicodeText, hMem); res == 0 {
		procGlobalFree.Call(hMem)
	}
}
