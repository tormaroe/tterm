package main

import (
	"fmt"
	"image/color"
	"os"
	"runtime"
	"strings"
	"time"
)

// ExecuteCommand parses and executes a user command against the custom Terminal.
func (t *Terminal) ExecuteCommand(cmdline string) {
	if cmdline == "" {
		return
	}

	fields := strings.Fields(cmdline)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "help", "?":
		t.PrintLine([]Span{{Text: "Available Commands:", FG: ColorInfo}})
		t.PrintLine([]Span{
			{Text: "  help              ", FG: ColorPrompt},
			{Text: "Show this help message", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  clear / cls       ", FG: ColorPrompt},
			{Text: "Clear the terminal screen (or press Ctrl+L)", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  echo <text>       ", FG: ColorPrompt},
			{Text: "Print text back to the terminal", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  colors            ", FG: ColorPrompt},
			{Text: "Show a 24-bit RGB color palette & styled text demo", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  sysinfo           ", FG: ColorPrompt},
			{Text: "Display Go runtime & OS information", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  stream            ", FG: ColorPrompt},
			{Text: "Run an async background goroutine streaming live output", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  exit / quit       ", FG: ColorPrompt},
			{Text: "Close the terminal window", FG: ColorFG},
		})
		t.PrintLine([]Span{{Text: "", FG: ColorFG}})
		t.PrintLine([]Span{{Text: "Keyboard & Mouse Shortcuts:", FG: ColorInfo}})
		t.PrintLine([]Span{
			{Text: "  Up / Down         ", FG: ColorWarn},
			{Text: "Cycle through command history", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  Ctrl+Left/Right   ", FG: ColorWarn},
			{Text: "Jump cursor by word", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  Mouse Drag        ", FG: ColorWarn},
			{Text: "Select text on screen (auto-copies on Ctrl+C)", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  Ctrl+V            ", FG: ColorWarn},
			{Text: "Paste text from Windows clipboard", FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "  Mouse Wheel       ", FG: ColorWarn},
			{Text: "Scroll through output history", FG: ColorFG},
		})

	case "clear", "cls":
		t.Clear()

	case "echo":
		t.PrintPlain(strings.Join(args, " "), ColorFG)

	case "sysinfo":
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		t.PrintLine([]Span{
			{Text: "Go Version:    ", FG: ColorMuted},
			{Text: runtime.Version(), FG: ColorInfo},
		})
		t.PrintLine([]Span{
			{Text: "OS / Arch:     ", FG: ColorMuted},
			{Text: fmt.Sprintf("%s / %s (%d CPUs)", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()), FG: ColorFG},
		})
		t.PrintLine([]Span{
			{Text: "Heap Memory:   ", FG: ColorMuted},
			{Text: fmt.Sprintf("%.2f MB allocated", float64(mem.Alloc)/(1024*1024)), FG: ColorPrompt},
		})
		t.PrintLine([]Span{
			{Text: "Current Time:  ", FG: ColorMuted},
			{Text: time.Now().Format("2006-01-02 15:04:05"), FG: ColorWarn},
		})

	case "colors":
		t.PrintLine([]Span{{Text: "24-bit TrueColor Gradient Demo:", FG: ColorInfo}})
		var gradient []Span
		steps := 48
		for i := 0; i < steps; i++ {
			r := uint8((i * 255) / steps)
			g := uint8(255 - ((i * 255) / steps))
			b := uint8(((i * 510) / steps) % 255)
			gradient = append(gradient, Span{
				Text: "█",
				FG:   color.RGBA{r, g, b, 255},
			})
		}
		t.PrintLine(gradient)
		t.PrintLine([]Span{
			{Text: "[SUCCESS] ", FG: ColorPrompt},
			{Text: "[INFO] ", FG: ColorInfo},
			{Text: "[WARNING] ", FG: ColorWarn},
			{Text: "[ERROR] ", FG: ColorError},
			{Text: "[MUTED] ", FG: ColorMuted},
		})

	case "stream":
		t.PrintLine([]Span{
			{Text: "[async] ", FG: ColorInfo},
			{Text: "Starting background worker goroutine (you can keep typing while it runs!)...", FG: ColorFG},
		})
		go func() {
			steps := []string{
				"Connecting to telemetry subsystem...",
				"Sampling sensor array [1/4]: OK (0.4ms)",
				"Sampling sensor array [2/4]: OK (0.3ms)",
				"Sampling sensor array [3/4]: OK (0.5ms)",
				"Sampling sensor array [4/4]: OK (0.2ms)",
				"Background task completed!",
			}
			for i, msg := range steps {
				time.Sleep(600 * time.Millisecond)
				fg := ColorFG
				if i == len(steps)-1 {
					fg = ColorPrompt
				}
				t.PrintLine([]Span{
					{Text: fmt.Sprintf("  [%s] ", time.Now().Format("15:04:05")), FG: ColorMuted},
					{Text: msg, FG: fg},
				})
			}
		}()

	case "exit", "quit":
		os.Exit(0)

	default:
		t.PrintLine([]Span{
			{Text: "Unknown command: ", FG: ColorError},
			{Text: cmd, FG: ColorWarn},
			{Text: " (type ", FG: ColorMuted},
			{Text: "help", FG: ColorInfo},
			{Text: " for a list of commands)", FG: ColorMuted},
		})
	}
}
