package main

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gomono"
)

const (
	baseFontSize = 16.0
	paddingX     = 12.0
	paddingY     = 10.0
)

// App implements the ebiten.Game interface.
type App struct {
	term       *Terminal
	fontSource *text.GoTextFaceSource
	fontFace   *text.GoTextFace

	// Measured monospace cell dimensions in device pixels
	cellWidth  float64
	cellHeight float64
	dpiScale   float64

	// Current viewport dimensions in device pixels
	screenWidth  int
	screenHeight int
}

// loadMonospaceFont loads Cascadia Mono or Consolas from Windows Fonts if available,
// falling back to Go's embedded GoMono font so it always works everywhere.
func loadMonospaceFont() *text.GoTextFaceSource {
	candidates := []string{
		`C:\Windows\Fonts\CascadiaMono.ttf`,
		`C:\Windows\Fonts\consola.ttf`,
	}
	for _, path := range candidates {
		if data, err := os.ReadFile(path); err == nil {
			if src, err := text.NewGoTextFaceSource(bytes.NewReader(data)); err == nil {
				return src
			}
		}
	}

	src, err := text.NewGoTextFaceSource(bytes.NewReader(gomono.TTF))
	if err != nil {
		log.Fatalf("failed to load fallback font: %v", err)
	}
	return src
}

func NewApp() *App {
	src := loadMonospaceFont()
	app := &App{
		term:       NewTerminal(),
		fontSource: src,
	}
	app.updateFontMetrics(1.0)
	return app
}

// updateFontMetrics recalculates the font face and monospace grid cell size for the monitor's DPI scale.
func (a *App) updateFontMetrics(scale float64) {
	if scale <= 0 {
		scale = 1.0
	}
	if math.Abs(a.dpiScale-scale) < 0.01 && a.fontFace != nil {
		return
	}
	a.dpiScale = scale
	scaledSize := math.Round(baseFontSize * scale)
	a.fontFace = &text.GoTextFace{
		Source: a.fontSource,
		Size:   scaledSize,
	}

	// Measure 'M' to get exact monospace cell width and line height
	w, h := text.Measure("M", a.fontFace, 0)
	a.cellWidth = math.Ceil(w)
	a.cellHeight = math.Ceil(h * 1.25)
}

// keyRepeating returns true on the initial frame a key is pressed, and after an initial delay,
// repeats every few ticks so holding Backspace or Arrow keys feels natural.
func keyRepeating(key ebiten.Key) bool {
	d := inpututil.KeyPressDuration(key)
	if d == 1 {
		return true
	}
	// Wait ~350ms (21 ticks at 60Hz), then repeat every ~33ms (2 ticks)
	if d >= 21 && (d-21)%2 == 0 {
		return true
	}
	return false
}

func (a *App) Update() error {
	a.term.BlinkTicks++

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)

	// 1. Handle Ctrl+<Key> shortcuts first so they don't insert characters
	if ctrl {
		if inpututil.IsKeyJustPressed(ebiten.KeyL) {
			a.term.Clear()
			return nil
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyC) {
			if sel := a.term.SelectedText(); sel != "" {
				SetClipboardText(sel)
				a.term.HasSelection = false
			} else {
				// Standard terminal Ctrl+C behavior when nothing is selected: cancel current input line
				a.term.mu.Lock()
				raw := string(a.term.Input)
				a.term.Input = nil
				a.term.CursorIndex = 0
				a.term.Lines = append(a.term.Lines, Line{
					Spans: []Span{
						{Text: a.term.PromptPrefix, FG: ColorPrompt},
						{Text: raw + "^C", FG: ColorError},
					},
				})
				a.term.mu.Unlock()
			}
			return nil
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyV) {
			clip := GetClipboardText()
			if clip != "" {
				a.term.InsertRunes([]rune(clip))
			}
			return nil
		}
		if keyRepeating(ebiten.KeyLeft) {
			a.term.MoveCursorWord(-1)
			return nil
		}
		if keyRepeating(ebiten.KeyRight) {
			a.term.MoveCursorWord(1)
			return nil
		}
	} else {
		// 2. Normal typed characters (handles layout, Shift, dead keys, Unicode)
		var runes []rune
		runes = ebiten.AppendInputChars(runes)
		if len(runes) > 0 {
			a.term.InsertRunes(runes)
			a.term.HasSelection = false
		}
	}

	// 3. Navigation and editing keys
	if keyRepeating(ebiten.KeyBackspace) {
		a.term.Backspace()
	}
	if keyRepeating(ebiten.KeyDelete) {
		a.term.Delete()
	}
	if !ctrl && keyRepeating(ebiten.KeyLeft) {
		a.term.MoveCursor(-1)
	}
	if !ctrl && keyRepeating(ebiten.KeyRight) {
		a.term.MoveCursor(1)
	}
	if keyRepeating(ebiten.KeyUp) {
		a.term.NavigateHistory(-1)
	}
	if keyRepeating(ebiten.KeyDown) {
		a.term.NavigateHistory(1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		a.term.MoveCursorHomeEnd(false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		a.term.MoveCursorHomeEnd(true)
	}

	// 4. Submit command on Enter / Numpad Enter
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		cmd := a.term.SubmitLine()
		a.term.ExecuteCommand(cmd)
	}

	// 5. Mouse wheel scrolling
	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		a.term.mu.Lock()
		a.term.ScrollLines += int(wheelY * 3)
		if a.term.ScrollLines < 0 {
			a.term.ScrollLines = 0
		}
		a.term.mu.Unlock()
	}

	// 6. Mouse click-and-drag text selection
	a.handleMouseSelection()

	return nil
}

func (a *App) handleMouseSelection() {
	if a.cellWidth <= 0 || a.cellHeight <= 0 {
		return
	}

	padX := paddingX * a.dpiScale
	padY := paddingY * a.dpiScale
	visibleRows := int((float64(a.screenHeight) - padY*2) / a.cellHeight)
	if visibleRows < 1 {
		visibleRows = 1
	}

	a.term.mu.Lock()
	totalRows := len(a.term.LastVisualRows)
	startRow := totalRows - visibleRows - a.term.ScrollLines
	if startRow < 0 {
		startRow = 0
	}
	a.term.mu.Unlock()

	mx, my := ebiten.CursorPosition()
	col := int((float64(mx) - padX) / a.cellWidth)
	if col < 0 {
		col = 0
	}
	row := startRow + int((float64(my)-padY)/a.cellHeight)
	if row < 0 {
		row = 0
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.term.mu.Lock()
		a.term.Selecting = true
		a.term.HasSelection = false
		a.term.SelectStart = GridPos{Row: row, Col: col}
		a.term.SelectEnd = GridPos{Row: row, Col: col}
		a.term.mu.Unlock()
	} else if a.term.Selecting && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		a.term.mu.Lock()
		a.term.SelectEnd = GridPos{Row: row, Col: col}
		if a.term.SelectEnd != a.term.SelectStart {
			a.term.HasSelection = true
		}
		a.term.mu.Unlock()
	} else if a.term.Selecting && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		a.term.mu.Lock()
		a.term.Selecting = false
		a.term.mu.Unlock()
	}

	// Right-click copies selection if active, otherwise pastes from clipboard
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		if sel := a.term.SelectedText(); sel != "" {
			SetClipboardText(sel)
			a.term.HasSelection = false
		} else if clip := GetClipboardText(); clip != "" {
			a.term.InsertRunes([]rune(clip))
		}
	}
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(ColorBG)

	padX := paddingX * a.dpiScale
	padY := paddingY * a.dpiScale

	usableW := float64(a.screenWidth) - padX*2
	usableH := float64(a.screenHeight) - padY*2
	if usableW <= 0 || usableH <= 0 || a.cellWidth <= 0 || a.cellHeight <= 0 {
		return
	}

	cols := int(usableW / a.cellWidth)
	visibleRows := int(usableH / a.cellHeight)
	if cols < 10 {
		cols = 10
	}
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Wrap lines to current column count
	visualRows, cursorRow, cursorCol := a.term.BuildVisualRows(cols)

	// Clamp scroll offset to available history
	maxScroll := len(visualRows) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	a.term.mu.Lock()
	if a.term.ScrollLines > maxScroll {
		a.term.ScrollLines = maxScroll
	}
	scroll := a.term.ScrollLines
	hasSel := a.term.HasSelection
	selStart, selEnd := NormalizeSelection(a.term.SelectStart, a.term.SelectEnd)
	blink := (a.term.BlinkTicks/30)%2 == 0
	a.term.mu.Unlock()

	startRow := len(visualRows) - visibleRows - scroll
	if startRow < 0 {
		startRow = 0
	}
	endRow := startRow + visibleRows
	if endRow > len(visualRows) {
		endRow = len(visualRows)
	}

	// Render visible rows
	for vIdx, rowIdx := 0, startRow; rowIdx < endRow; vIdx, rowIdx = vIdx+1, rowIdx+1 {
		row := visualRows[rowIdx]
		y := padY + float64(vIdx)*a.cellHeight

		// 1. Draw selection background if any cells in this row are selected
		if hasSel && rowIdx >= selStart.Row && rowIdx <= selEnd.Row {
			cStart := 0
			cEnd := cols
			if rowIdx == selStart.Row {
				cStart = selStart.Col
			}
			if rowIdx == selEnd.Row {
				cEnd = selEnd.Col + 1
			}
			if cStart < 0 {
				cStart = 0
			}
			if cEnd > cols {
				cEnd = cols
			}
			if cEnd > cStart {
				selX := float32(padX + float64(cStart)*a.cellWidth)
				selW := float32(float64(cEnd-cStart) * a.cellWidth)
				vector.DrawFilledRect(screen, selX, float32(y), selW, float32(a.cellHeight), ColorSelection, false)
			}
		}

		// 2. Batch contiguous same-color cells on this row into a single Draw call for performance
		colIdx := 0
		for colIdx < len(row) {
			fg := row[colIdx].FG
			runStart := colIdx
			var runRunes []rune
			for colIdx < len(row) && row[colIdx].FG == fg {
				runRunes = append(runRunes, row[colIdx].Char)
				colIdx++
			}

			x := padX + float64(runStart)*a.cellWidth
			op := &text.DrawOptions{}
			op.GeoM.Translate(x, y)
			op.ColorScale.ScaleWithColor(fg)
			text.Draw(screen, string(runRunes), a.fontFace, op)
		}
	}

	// 3. Draw blinking block cursor if visible inside viewport
	if blink && cursorRow >= startRow && cursorRow < endRow {
		curVRow := cursorRow - startRow
		cx := float32(padX + float64(cursorCol)*a.cellWidth)
		cy := float32(padY + float64(curVRow)*a.cellHeight)
		vector.DrawFilledRect(screen, cx, cy, float32(a.cellWidth), float32(a.cellHeight), ColorCursor, false)

		// If cursor is sitting over a character, draw that character inverted in dark color
		if cursorRow < len(visualRows) && cursorCol < len(visualRows[cursorRow]) {
			ch := visualRows[cursorRow][cursorCol].Char
			op := &text.DrawOptions{}
			op.GeoM.Translate(float64(cx), float64(cy))
			op.ColorScale.ScaleWithColor(ColorBG)
			text.Draw(screen, string(ch), a.fontFace, op)
		}
	}

	// 4. Draw scrollback indicator pill if scrolled up
	if scroll > 0 {
		msg := fmt.Sprintf(" ↑ Scrolled up %d lines (Wheel down or type to return) ", scroll)
		ebitenutil.DebugPrintAt(screen, msg, int(padX), 2)
	}
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	a.updateFontMetrics(scale)

	a.screenWidth = int(float64(outsideWidth) * scale)
	a.screenHeight = int(float64(outsideHeight) * scale)
	return a.screenWidth, a.screenHeight
}

func main() {
	ebiten.SetWindowSize(960, 600)
	ebiten.SetWindowTitle("tterm — Custom Go + Ebitengine Terminal")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	app := NewApp()
	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}
