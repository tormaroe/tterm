# tterm

A custom, standalone terminal UI and interactive command shell written from scratch in **Go** using **[Ebitengine](https://ebitengine.org/)** (`v2`).

Unlike traditional terminal emulators that wrap OS pseudo-terminals (`ConPTY` / `conhost.exe`) and parse legacy VT100/ANSI escape sequences, `tterm` renders its own GPU-accelerated monospace grid, manages its own line editor and scrollback buffer, and executes custom Go commands directly in-process.

---

## Prerequisites

- **OS:** Windows (10/11)
- **Go:** `go1.27+`
  - Pure Go build — **no C compiler (`cgo`) required**.

---

## Build & Run Instructions

Open PowerShell or Command Prompt in the repository root:

### 1. Run directly during development
```powershell
go run .
```

### 2. Build a standalone GUI executable (`tterm.exe`)
Using `-ldflags="-H=windowsgui"` compiles `tterm.exe` as a pure Windows GUI application so it launches directly without opening a background `cmd`/`conhost` console window:

```powershell
go build -ldflags="-H=windowsgui" -o tterm.exe .
```

Then launch it from PowerShell or double-click `tterm.exe` in File Explorer:
```powershell
.\tterm.exe
```

### 3. Update dependencies (if adding new packages)
```powershell
go mod tidy
```

---

## Keyboard & Mouse Controls

| Input | Action |
| :--- | :--- |
| `Left` / `Right` | Move cursor one character left or right |
| `Ctrl+Left` / `Ctrl+Right` | Jump cursor by word |
| `Home` / `End` | Jump cursor to the start or end of the input line |
| `Up` / `Down` | Cycle through previous command history |
| `Backspace` / `Delete` | Delete character before or under the cursor |
| `Enter` | Submit and execute the current command |
| `Ctrl+L` | Clear the terminal scrollback buffer |
| `Mouse Left-Drag` | Highlight and select text on screen |
| `Ctrl+C` | Copy selected text to clipboard (or cancel current input if nothing is selected) |
| `Ctrl+V` | Paste text from Windows clipboard at cursor |
| `Mouse Right-Click` | Copy selection if active; otherwise paste from clipboard |
| `Mouse Wheel` | Scroll up and down through output history |

---

## Project Structure

- **`main.go`** — Ebitengine application loop (`Update`, `Draw`, `Layout`), High-DPI font loading (`CascadiaMono.ttf` → `consola.ttf` → embedded `gomono.TTF` fallback), batched GPU text rendering, and input polling.
- **`terminal.go`** — Thread-safe `Terminal` state: styled text `Span`/`Line` scrollback buffer, automatic line wrapping on window resize, readline-style prompt editor, mouse selection math, and pure-Go Win32 clipboard bindings.
- **`commands.go`** — Custom command dispatcher (`ExecuteCommand`). Add your own commands and background goroutine tasks here.
