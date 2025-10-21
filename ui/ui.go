package ui

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"0xADE/xpass/config"
	"0xADE/xpass/passcard"
	"0xADE/xpass/storage"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/richtext"

	"github.com/atotto/clipboard"
)

type fieldWidget struct {
	editor         widget.Editor
	clickable      widget.Clickable
	labelClickable widget.Clickable
}

type UI struct {
	storage       *storage.Storage
	config        *config.Config
	theme         *material.Theme
	window        *app.Window
	searchEditor  widget.Editor
	list          widget.List
	query         string
	filtered      []passcard.StoredItem
	selectedIdx   int
	status        string
	countingDown  bool
	countdown     float32
	countdownDone chan bool
	statusMutex   sync.RWMutex

	initialized          bool
	metadataState        richtext.InteractiveText
	metadataEditor       widget.Editor
	showRichText         bool
	metadataAreaClick    widget.Clickable
	toggleButton         widget.Clickable
	lastMetadataText     string
	lastMetadataItemIdx  int
	lastMetadataRichMode bool

	// Rich mode field widgets
	fieldWidgets map[string]*fieldWidget
	kvPairs      []KeyValuePair
	markdownText string

	// Filter debouncing
	queryInput   chan string
	queryResults chan []passcard.StoredItem
	stopFilter   chan struct{}

	// Key repeat handling
	keyRepeatActive bool
	keyRepeatName   key.Name
	keyRepeatStart  time.Time

	// Edit mode
	editMode       bool
	editModeEditor widget.Editor
	modifyButton   widget.Clickable
	saveButton     widget.Clickable
	cancelButton   widget.Clickable
	passgenButton  widget.Clickable
	// Create mode
	createMode   bool
	createEditor widget.Editor
	addButton    widget.Clickable
}

func New(store *storage.Storage, cfg *config.Config) *UI {
	ui := &UI{
		storage:       store,
		config:        cfg,
		countdownDone: make(chan bool, 1),
		list: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
		showRichText:        true,
		lastMetadataItemIdx: -1, // Force initial update
		fieldWidgets:        make(map[string]*fieldWidget),
		queryInput:          make(chan string, 64),
		queryResults:        make(chan []passcard.StoredItem, 1),
		stopFilter:          make(chan struct{}),
	}

	ui.searchEditor.SingleLine = true
	ui.searchEditor.Submit = true

	ui.metadataEditor.ReadOnly = true
	ui.metadataEditor.SingleLine = false

	ui.editModeEditor.SingleLine = false
	ui.editModeEditor.ReadOnly = false

	ui.createEditor.SingleLine = true
	ui.createEditor.Submit = true

	store.Subscribe(func(status string) {
		ui.statusMutex.Lock()
		ui.status = status
		ui.statusMutex.Unlock()
		ui.updateQuery()
		if ui.window != nil {
			ui.window.Invalidate()
		}
	})

	ui.updateQuery()
	ui.startFilterWorker()
	return ui
}

func (ui *UI) startFilterWorker() {
	go func() {
		var timer *time.Timer
		var latestQuery string
		debounceDelay := 50 * time.Millisecond

		for {
			select {
			case query := <-ui.queryInput:
				latestQuery = query
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounceDelay, func() {
					filtered := ui.storage.Query(latestQuery)
					select {
					case ui.queryResults <- filtered:
						if ui.window != nil {
							ui.window.Invalidate()
						}
					default:
						// Skip if channel full
					}
				})
			case <-ui.stopFilter:
				if timer != nil {
					timer.Stop()
				}
				return
			}
		}
	}()
}

func (ui *UI) updateQuery() {
	ui.filtered = ui.storage.Query(ui.query)
	if ui.selectedIdx >= len(ui.filtered) {
		ui.selectedIdx = 0
	}
}

func (ui *UI) moveSelectionUp() {
	if ui.selectedIdx > 0 {
		ui.selectedIdx--
		if ui.list.Position.First > ui.selectedIdx {
			ui.list.Position.First = ui.selectedIdx
		}
	}
}

func (ui *UI) moveSelectionDown() {
	if ui.selectedIdx < len(ui.filtered)-1 {
		ui.selectedIdx++
		if ui.list.Position.Count > 0 && ui.list.Position.First+ui.list.Position.Count <= ui.selectedIdx {
			ui.list.Position.First = ui.selectedIdx - ui.list.Position.Count + 1
		}
	}
}

func (ui *UI) copyToClipboard() {
	if ui.selectedIdx >= len(ui.filtered) {
		ui.statusMutex.Lock()
		ui.status = "No password selected"
		ui.statusMutex.Unlock()
		return
	}

	pw := ui.filtered[ui.selectedIdx]
	pass := pw.Password()
	if err := clipboard.WriteAll(pass); err != nil {
		ui.statusMutex.Lock()
		ui.status = fmt.Sprintf("Failed to copy: %v", err)
		ui.statusMutex.Unlock()
		return
	}

	ui.statusMutex.Lock()
	ui.status = "Copied to clipboard"
	ui.statusMutex.Unlock()
	go ui.clearClipboard()
}

func (ui *UI) copyFieldToClipboard(value string) {
	if err := clipboard.WriteAll(value); err != nil {
		ui.statusMutex.Lock()
		ui.status = fmt.Sprintf("Failed to copy: %v", err)
		ui.statusMutex.Unlock()
		return
	}

	ui.statusMutex.Lock()
	ui.status = "Copied to clipboard"
	ui.statusMutex.Unlock()
	go ui.clearClipboard()
}

func (ui *UI) findFieldValue(keys ...string) string {
	for _, key := range keys {
		keyLower := strings.ToLower(key)
		for _, pair := range ui.kvPairs {
			if strings.ToLower(pair.Key) == keyLower {
				return pair.Value
			}
		}
	}
	return ""
}

func (ui *UI) copyFieldByKeys(keys ...string) {
	value := ui.findFieldValue(keys...)
	if value == "" {
		ui.statusMutex.Lock()
		ui.status = fmt.Sprintf("Field not found: %v", keys)
		ui.statusMutex.Unlock()
		return
	}
	ui.copyFieldToClipboard(value)
}

func (ui *UI) openURL(url string) {
	if url == "" {
		ui.statusMutex.Lock()
		ui.status = "No URL found"
		ui.statusMutex.Unlock()
		return
	}

	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err != nil {
		ui.statusMutex.Lock()
		ui.status = fmt.Sprintf("Failed to open URL: %v", err)
		ui.statusMutex.Unlock()
		return
	}

	ui.statusMutex.Lock()
	ui.status = fmt.Sprintf("Opening %s", url)
	ui.statusMutex.Unlock()
}

func (ui *UI) enterEditMode() {
	fmt.Println("DEBUG: enterEditMode() called")
	if ui.selectedIdx >= len(ui.filtered) {
		fmt.Println("DEBUG: selectedIdx out of range")
		return
	}

	// Get the current item
	item := ui.filtered[ui.selectedIdx]
	fmt.Printf("DEBUG: Entering edit mode for: %s\n", item.Name)

	// Decrypt to get full content
	decrypted, ok := item.Storage.GetCached(item.Path)
	if !ok || decrypted == "" {
		ui.statusMutex.Lock()
		ui.status = "Cannot edit: decrypt first"
		ui.statusMutex.Unlock()
		fmt.Println("DEBUG: Cannot edit - not decrypted")
		return
	}
	fmt.Printf("DEBUG: Decrypted content length: %d\n", len(decrypted))

	// Set editor text to full content (password + metadata)
	ui.editModeEditor.SetText(decrypted)
	ui.editMode = true
	fmt.Println("DEBUG: Edit mode activated successfully")

	// Request focus for edit mode editor on next frame
	if ui.window != nil {
		ui.window.Invalidate()
	}
}

func (ui *UI) saveEditMode() {
	fmt.Println("DEBUG: saveEditMode called")
	if ui.selectedIdx >= len(ui.filtered) {
		fmt.Println("DEBUG: selectedIdx out of range")
		return
	}

	item := ui.filtered[ui.selectedIdx]
	fmt.Printf("DEBUG: Saving item: %s\n", item.Name)

	// Get full content from editor
	content := ui.editModeEditor.Text()
	fmt.Printf("DEBUG: Content length: %d\n", len(content))

	// Get GPG recipient(s)
	gpgIDs := ui.getGPGRecipients()

	if len(gpgIDs) == 0 {
		ui.statusMutex.Lock()
		ui.status = "No GPG key configured"
		ui.statusMutex.Unlock()
		fmt.Println("DEBUG: No GPG key configured")
		return
	}
	fmt.Printf("DEBUG: Using GPG IDs: %v\n", gpgIDs)

	// Encrypt with GPG - add all recipients
	args := []string{"--encrypt", "--batch", "--yes", "--output", item.Path, "--armor"}
	for _, gpgID := range gpgIDs {
		args = append(args, "--recipient", gpgID)
	}

	cmd := exec.Command("gpg", args...)
	cmd.Stdin = strings.NewReader(content)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	fmt.Printf("DEBUG: Running GPG command: gpg %v\n", args)
	if err := cmd.Run(); err != nil {
		ui.statusMutex.Lock()
		ui.status = fmt.Sprintf("Failed to save: %v", err)
		ui.statusMutex.Unlock()
		fmt.Printf("DEBUG: GPG error: %v\nStderr: %s\n", err, stderr.String())
		return
	}
	fmt.Println("DEBUG: GPG encryption successful")

	// Invalidate cache and update
	if item.Storage != nil {
		item.Storage.SetCached(item.Path, content)
	}

	ui.editMode = false
	ui.statusMutex.Lock()
	ui.status = "Saved successfully"
	ui.statusMutex.Unlock()

	// Force re-extract kvPairs
	ui.lastMetadataItemIdx = -1
	ui.updateQuery()

	// Request focus back to search editor
	if ui.window != nil {
		ui.window.Invalidate()
	}
}

func (ui *UI) cancelEditMode() {
	ui.editMode = false
	ui.statusMutex.Lock()
	ui.status = "Edit cancelled"
	ui.statusMutex.Unlock()

	// Request focus back to search editor
	if ui.window != nil {
		ui.window.Invalidate()
	}
}

func (ui *UI) getGPGRecipients() []string {
	var gpgIDs []string
	if ui.config.PasswordStoreKey != "" {
		return []string{ui.config.PasswordStoreKey}
	}
	// Try reading .gpg-id file from password store
	gpgIDPath := filepath.Join(ui.storage.Path(), ".gpg-id")
	fmt.Printf("DEBUG: Reading GPG ID from: %s\n", gpgIDPath)
	data, err := os.ReadFile(gpgIDPath)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				gpgIDs = append(gpgIDs, line)
			}
		}
	} else {
		fmt.Printf("DEBUG: Error reading .gpg-id: %v\n", err)
	}
	return gpgIDs
}

func (ui *UI) createNewPassword() {
	name := ui.createEditor.Text()
	if name == "" {
		ui.status = "Password name cannot be empty"
		return
	}
	name = strings.TrimSuffix(name, ".gpg")

	gpgIDs := ui.getGPGRecipients()
	if len(gpgIDs) == 0 {
		ui.statusMutex.Lock()
		ui.status = "No GPG key configured"
		ui.statusMutex.Unlock()
		return
	}

	fullPath, err := ui.storage.Create(name, "\n", gpgIDs)
	if err != nil {
		ui.statusMutex.Lock()
		ui.status = fmt.Sprintf("Failed to create: %v", err)
		ui.statusMutex.Unlock()
		return
	}

	ui.createMode = false
	ui.statusMutex.Lock()
	ui.status = "Created successfully"
	ui.statusMutex.Unlock()

	// The watcher in storage should have updated the list.
	// We call updateQuery to be safe and to get the new list immediately.
	ui.updateQuery()

	found := false
	for i, item := range ui.filtered {
		if item.Path == fullPath {
			ui.selectedIdx = i
			found = true
			break
		}
	}

	if found {
		ui.enterEditMode()
	} else {
		ui.statusMutex.Lock()
		ui.status = "Could not select new password"
		ui.statusMutex.Unlock()
	}
}

func (ui *UI) clearClipboard() {
	ui.statusMutex.Lock()
	if ui.countdown > 0 {
		ui.countdown = float32(ui.config.PasswordStoreClipSeconds)
		ui.statusMutex.Unlock()
		return
	}
	if ui.countingDown {
		ui.statusMutex.Unlock()
		ui.countdownDone <- true
		return
	}
	ui.countingDown = true
	ui.statusMutex.Unlock()

	tick := 200 * time.Millisecond
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	remaining := float64(ui.config.PasswordStoreClipSeconds)
	for {
		select {
		case <-ui.countdownDone:
			ui.statusMutex.Lock()
			ui.countingDown = false
			ui.statusMutex.Unlock()
			return
		case <-ticker.C:
			ui.statusMutex.Lock()
			ui.countdown = float32(remaining / float64(ui.config.PasswordStoreClipSeconds))
			ui.status = fmt.Sprintf("Will clear %s in %.0f seconds", ui.storage.NameByIdx(ui.selectedIdx), remaining)
			ui.statusMutex.Unlock()
			if ui.window != nil {
				ui.window.Invalidate()
			}
			remaining -= tick.Seconds()
			if remaining <= 0 {
				clipboard.WriteAll("")
				ui.statusMutex.Lock()
				ui.status = "Clipboard cleared"
				ui.countingDown = false
				ui.statusMutex.Unlock()
				if ui.window != nil {
					ui.window.Invalidate()
				}
				return
			}
		}
	}
}

func generatePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	const length = 16

	password := make([]byte, length)
	randomBytes := make([]byte, length)

	if _, err := rand.Read(randomBytes); err != nil {
		return ""
	}

	for i := 0; i < length; i++ {
		password[i] = charset[int(randomBytes[i])%len(charset)]
	}

	return string(password)
}

func (ui *UI) Run() error {
	ui.window = new(app.Window)
	ui.window.Option(app.Title("xpass"))
	ui.window.Option(app.Size(unit.Dp(1080), unit.Dp(920)))

	go func() {
		if err := ui.loop(); err != nil {
			panic(err)
		}
	}()

	app.Main()
	return nil
}

func (ui *UI) loop() error {
	th := material.NewTheme()
	ui.theme = th

	var ops op.Ops
	for {
		switch e := ui.window.Event().(type) {
		case app.DestroyEvent:
			close(ui.stopFilter)
			return e.Err

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// Process filter results
			select {
			case filtered := <-ui.queryResults:
				ui.filtered = filtered
				if ui.selectedIdx >= len(ui.filtered) {
					ui.selectedIdx = 0
				}
			default:
				// No results ready
			}

			if !ui.initialized {
				gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
				ui.initialized = true
			}

			// Focus search editor when not in edit mode
			if !ui.editMode && !ui.createMode {
				gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
			}

			// Register global key listener
			area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			event.Op(gtx.Ops, ui.window)

			// Handle lobal keyboard shortcuts ============================
			//
			// ,---,---,---,---,---,---,---,---,---,---,---,---,---,-------,
			// |1/2| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 0 | + | ' | <-    |
			// |---'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-----|
			// | ->| | Q | W | E | R | T | Y | U | I | O | P | ] | ^ |     |
			// |-----',--',--',--',--',--',--',--',--',--',--',--',--'|    |
			// | Caps | A | S | D | F | G | H | J | K | L | \ | [ | * |    |
			// |----,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'-,-'---'----|
			// |    | < | Z | X | C | V | B | N | M | , | . | - |          |
			// |----'-,-',--'--,'---'---'---'---'---'---'-,-'---',--,------|
			// | ctrl |  | alt |                          |altgr |  | ctrl |
			// '------'  '-----'--------------------------'------'  '------'
			//

			// Build key filters based on edit mode
			var filters []event.Filter
			filters = append(filters, key.Filter{Name: key.NameEscape})

			// Don't filter arrow keys in edit mode - let the editor handle them
			if !ui.editMode && !ui.createMode {
				filters = append(filters,
					key.Filter{Name: key.NameUpArrow},
					key.Filter{Name: key.NameDownArrow},
				)
			}

			filters = append(filters,
				key.Filter{Name: "T"},
				key.Filter{Name: "C"},
				key.Filter{Name: "L"},
				key.Filter{Name: "E"},
				key.Filter{Name: "O"},
				key.Filter{Name: "M"},
			)

			for {
				ev, ok := gtx.Event(filters...)
				if !ok {
					break
				}
				if kev, ok := ev.(key.Event); ok {
					if kev.State == key.Press {
						switch kev.Name {
						case key.NameEscape:
							if ui.createMode {
								ui.createMode = false
								gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
							} else if ui.editMode {
								ui.cancelEditMode()
							} else {
								os.Exit(0)
							}
						case key.NameUpArrow:
							// Don't handle arrow keys in edit mode - let the editor handle them
							if !ui.editMode && !ui.createMode {
								ui.moveSelectionUp()
								ui.keyRepeatActive = true
								ui.keyRepeatName = key.NameUpArrow
								ui.keyRepeatStart = time.Now()
							}
						case key.NameDownArrow:
							// Don't handle arrow keys in edit mode - let the editor handle them
							if !ui.editMode && !ui.createMode {
								ui.moveSelectionDown()
								ui.keyRepeatActive = true
								ui.keyRepeatName = key.NameDownArrow
								ui.keyRepeatStart = time.Now()
							}
						case "T":
							if kev.Modifiers.Contain(key.ModCtrl) {
								ui.showRichText = !ui.showRichText
							}
						case "C":
							if kev.Modifiers.Contain(key.ModCtrl) {
								// Copy selected text from metadata editor or focused field
								if !ui.showRichText && ui.metadataEditor.Text() != "" {
									// In plain text mode, copy selection if any
									start, end := ui.metadataEditor.Selection()
									if start != end {
										if start > end {
											start, end = end, start
										}
										text := ui.metadataEditor.Text()
										if start < len(text) && end <= len(text) {
											selectedText := text[start:end]
											ui.copyFieldToClipboard(selectedText)
										}
									}
								}
							}
						case "L":
							if kev.Modifiers.Contain(key.ModCtrl) {
								ui.copyFieldByKeys("login", "user", "username")
							}
						case "E":
							if kev.Modifiers.Contain(key.ModCtrl) {
								ui.copyFieldByKeys("email", "mail", "e-mail")
							}
						case "O":
							if kev.Modifiers.Contain(key.ModCtrl) {
								// Try to find URL field first
								url := ui.findFieldValue("url", "link")
								if url == "" {
									// Look for any field value starting with protocol
									for _, pair := range ui.kvPairs {
										if strings.Contains(pair.Value, "://") {
											url = pair.Value
											break
										}
									}
								}
								if url != "" {
									ui.openURL(url)
								} else {
									ui.statusMutex.Lock()
									ui.status = "No URL found"
									ui.statusMutex.Unlock()
								}
							}
						case "M":
							fmt.Printf("DEBUG: M key pressed, modifiers: %v, editMode: %v\n", kev.Modifiers, ui.editMode)
							if kev.Modifiers.Contain(key.ModCtrl) {
								fmt.Println("DEBUG: Ctrl+M detected, calling enterEditMode()")
								ui.enterEditMode()
							}
						}
					} else if kev.State == key.Release {
						// Stop key repeat when key is released
						if ui.keyRepeatActive && kev.Name == ui.keyRepeatName {
							ui.keyRepeatActive = false
						}
					}
				}
			}

			// Handle key repeat for arrow keys (only when not in edit mode)
			if ui.keyRepeatActive && !ui.editMode && !ui.createMode {
				elapsed := time.Since(ui.keyRepeatStart)
				initialDelay := 200 * time.Millisecond
				repeatInterval := 30 * time.Millisecond

				if elapsed > initialDelay {
					repeatCount := int((elapsed - initialDelay) / repeatInterval)
					lastRepeatTime := initialDelay + time.Duration(repeatCount)*repeatInterval
					nextRepeatTime := lastRepeatTime + repeatInterval

					if elapsed >= nextRepeatTime {
						switch ui.keyRepeatName {
						case key.NameUpArrow:
							ui.moveSelectionUp()
						case key.NameDownArrow:
							ui.moveSelectionDown()
						}
						ui.keyRepeatStart = ui.keyRepeatStart.Add(nextRepeatTime)
					}

					// Schedule next frame
					gtx.Execute(op.InvalidateCmd{})
				} else {
					// Wait for initial delay
					gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(initialDelay - elapsed)})
				}
			} else if (ui.editMode || ui.createMode) && ui.keyRepeatActive {
				// Stop key repeat when entering edit mode
				ui.keyRepeatActive = false
			}
			// ===== END OF KEYBOARD HANDLING =====

			// Don't process search editor events when in edit or create mode
			if !ui.editMode && !ui.createMode {
				for {
					ev, ok := ui.searchEditor.Update(gtx)
					if !ok {
						break
					}
					switch ev.(type) {
					case widget.ChangeEvent:
						ui.query = ui.searchEditor.Text()
						fmt.Printf("DEBUG: Search editor changed, new text: %q\n", ui.query)
						select {
						case ui.queryInput <- ui.query:
						default:
							// Channel full, skip this update
						}
					case widget.SubmitEvent:
						ui.copyToClipboard()
					}
				}
			}

			if ui.createMode {
				for {
					ev, ok := ui.createEditor.Update(gtx)
					if !ok {
						break
					}
					switch ev.(type) {
					case widget.SubmitEvent:
						ui.createNewPassword()
					}
				}
			}

			ui.layout(gtx)
			area.Pop()
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, ui.layoutLeftPane),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(ui.layoutRightPane),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ui.statusMutex.RLock()
			countingDown := ui.countingDown
			ui.statusMutex.RUnlock()
			if countingDown {
				return ui.layoutProgressBar(gtx)
			}
			return layout.Dimensions{}
		}),
	)
}

func (ui *UI) layoutLeftPane(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				editor := material.Editor(ui.theme, &ui.searchEditor, "Search passwords...")
				editor.TextSize = unit.Sp(20)
				return editor.Layout(gtx)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, ui.layoutPasswordList)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ui.statusMutex.RLock()
				status := ui.status
				ui.statusMutex.RUnlock()
				label := material.Body2(ui.theme, status)
				label.Color = color.NRGBA{R: 170, G: 170, B: 170, A: 255}
				return label.Layout(gtx)
			})
		}),
	)
}

func (ui *UI) layoutPasswordList(gtx layout.Context) layout.Dimensions {
	return material.List(ui.theme, &ui.list).Layout(gtx, len(ui.filtered), func(gtx layout.Context, index int) layout.Dimensions {
		isSelected := index == ui.selectedIdx

		// First render the content to get its height
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(ui.theme, ui.filtered[index].Name)
			label.TextSize = unit.Sp(18)
			return label.Layout(gtx)
		})
		call := macro.Stop()

		// Draw background if selected, using full width
		if isSelected {
			selectionColor := color.NRGBA{R: 100, G: 150, B: 200, A: 100}
			bgRect := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
			defer clip.Rect{Max: bgRect}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: selectionColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}

		// Draw the content on top
		call.Add(gtx.Ops)
		return dims
	})
}

func (ui *UI) layoutToggleButton(gtx layout.Context) layout.Dimensions {
	if ui.toggleButton.Clicked(gtx) {
		ui.showRichText = !ui.showRichText
	}

	buttonText := "Not Masked Source"
	if !ui.showRichText {
		buttonText = "Formatted View"
	}

	btn := material.Button(ui.theme, &ui.toggleButton, buttonText)
	btn.TextSize = unit.Sp(14)
	btn.Background = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	btn.Color = color.NRGBA{R: 220, G: 220, B: 220, A: 255}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(180))
		return btn.Layout(gtx)
	})
}

func (ui *UI) layoutModifyButton(gtx layout.Context) layout.Dimensions {
	if ui.modifyButton.Clicked(gtx) {
		ui.enterEditMode()
	}

	btn := material.Button(ui.theme, &ui.modifyButton, "Modify")
	btn.TextSize = unit.Sp(14)
	btn.Background = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	btn.Color = color.NRGBA{R: 220, G: 220, B: 220, A: 255}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(100))
		return btn.Layout(gtx)
	})
}

func (ui *UI) layoutEditModeButtons(gtx layout.Context) layout.Dimensions {
	// Check for button clicks
	for ui.passgenButton.Clicked(gtx) {
		fmt.Println("DEBUG: Passgen button clicked")
		newPassword := generatePassword()
		if newPassword != "" {
			// Get current text
			currentText := ui.editModeEditor.Text()
			// Replace first line with new password
			lines := strings.SplitN(currentText, "\n", 2)
			if len(lines) > 1 {
				// Has metadata, keep it
				ui.editModeEditor.SetText(newPassword + "\n" + lines[1])
			} else {
				// Only password, replace it
				ui.editModeEditor.SetText(newPassword)
			}
			ui.statusMutex.Lock()
			ui.status = "Password generated"
			ui.statusMutex.Unlock()
		}
	}
	for ui.saveButton.Clicked(gtx) {
		fmt.Println("DEBUG: Save button clicked")
		ui.saveEditMode()
	}
	for ui.cancelButton.Clicked(gtx) {
		fmt.Println("DEBUG: Cancel button clicked")
		ui.cancelEditMode()
	}

	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			passgenBtn := material.Button(ui.theme, &ui.passgenButton, "Passgen")
			passgenBtn.TextSize = unit.Sp(14)
			passgenBtn.Background = color.NRGBA{R: 80, G: 120, B: 180, A: 255}
			passgenBtn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			return passgenBtn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			saveBtn := material.Button(ui.theme, &ui.saveButton, "Save")
			saveBtn.TextSize = unit.Sp(14)
			saveBtn.Background = color.NRGBA{R: 50, G: 150, B: 50, A: 255}
			saveBtn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			return saveBtn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			cancelBtn := material.Button(ui.theme, &ui.cancelButton, "Cancel")
			cancelBtn.TextSize = unit.Sp(14)
			cancelBtn.Background = color.NRGBA{R: 150, G: 50, B: 50, A: 255}
			cancelBtn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			return cancelBtn.Layout(gtx)
		}),
	)
}

func (ui *UI) layoutRightPane(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Max.X = gtx.Dp(unit.Dp(600))
	gtx.Constraints.Min.X = gtx.Dp(unit.Dp(300))

	return layout.Stack{layout.NE}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			rect := clip.Rect{Max: gtx.Constraints.Max}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 68, G: 68, B: 68, A: 255}, rect.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						ui.statusMutex.RLock()
						countingDown := ui.countingDown
						ui.statusMutex.RUnlock()
						if countingDown {
							return ui.layoutCountdown(gtx)
						}
						return layout.Dimensions{}
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ui.selectedIdx < len(ui.filtered) {
							name := ui.filtered[ui.selectedIdx].Name
							label := material.H6(ui.theme, name)
							label.Color = color.NRGBA{R: 238, G: 238, B: 238, A: 255}
							label.Alignment = text.Middle
							return label.Layout(gtx)
						}
						return layout.Dimensions{}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ui.editMode {
							// Show Save and Cancel buttons in edit mode
							return ui.layoutEditModeButtons(gtx)
						}
						// Show Toggle and Modify buttons when not in edit mode
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEvenly}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return ui.layoutToggleButton(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return ui.layoutModifyButton(gtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if ui.selectedIdx < len(ui.filtered) {
							// Show edit mode if active
							if ui.editMode {
								// Request focus for edit mode editor
								gtx.Execute(key.FocusCmd{Tag: &ui.editModeEditor})

								// Render with white background and gray border
								return layout.Stack{}.Layout(gtx,
									layout.Expanded(func(gtx layout.Context) layout.Dimensions {
										// White background
										rect := clip.Rect{Max: gtx.Constraints.Max}
										paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, rect.Op())
										return layout.Dimensions{Size: gtx.Constraints.Max}
									}),
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										// Gray border
										borderWidth := gtx.Dp(unit.Dp(2))
										borderColor := color.NRGBA{R: 128, G: 128, B: 128, A: 255}

										// Draw border
										defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
										paint.ColorOp{Color: borderColor}.Add(gtx.Ops)
										paint.PaintOp{}.Add(gtx.Ops)

										// Draw inner white area
										innerRect := clip.Rect{
											Min: image.Pt(borderWidth, borderWidth),
											Max: image.Pt(gtx.Constraints.Max.X-borderWidth, gtx.Constraints.Max.Y-borderWidth),
										}
										defer innerRect.Push(gtx.Ops).Pop()
										paint.ColorOp{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}.Add(gtx.Ops)
										paint.PaintOp{}.Add(gtx.Ops)

										// Content with padding
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											editor := material.Editor(ui.theme, &ui.editModeEditor, "")
											editor.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
											editor.Font.Typeface = font.Typeface("monospace")
											editor.TextSize = unit.Sp(20)
											return editor.Layout(gtx)
										})
									}),
								)
							}

							fullContent := ui.filtered[ui.selectedIdx].FullContent()
							if fullContent == "" {
								fullContent = "Press Enter to decrypt"
							}

							// Handle clicks in rich text mode
							if ui.showRichText {
								// Split password from metadata
								var password string
								var metadata string
								if fullContent != "" && fullContent != "Press Enter to decrypt" {
									lines := strings.SplitN(fullContent, "\n", 2)
									password = strings.TrimSpace(lines[0])
									if len(lines) > 1 {
										metadata = strings.TrimSpace(lines[1])
									}
								}

								// Mark that we're in rich text mode and track content
								if ui.selectedIdx != ui.lastMetadataItemIdx || fullContent != ui.lastMetadataText {
									ui.lastMetadataItemIdx = ui.selectedIdx
									ui.lastMetadataText = fullContent
									// Extract key-value pairs from metadata only
									ui.kvPairs, ui.markdownText = ExtractKeyValuePairs(metadata)
								}
								ui.lastMetadataRichMode = true

								// Rich text mode with input widgets for key-value pairs
								children := []layout.FlexChild{}

								// Add password field first
								if fullContent != "Press Enter to decrypt" {
									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return ui.layoutPasswordField(gtx, password)
									}))
									if len(ui.kvPairs) > 0 || ui.markdownText != "" {
										children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
									}
								}

								// Add key-value fields (not clickable for mode switching)
								for i, pair := range ui.kvPairs {
									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return ui.layoutKeyValueField(gtx, pair)
									}))
									if i < len(ui.kvPairs)-1 {
										children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
									}
								}

								// Add spacing if both sections exist
								if len(ui.kvPairs) > 0 && ui.markdownText != "" {
									children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout))
								}

								// Add markdown section (clickable for mode switching)
								if ui.markdownText != "" {
									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										// Check for click to switch to plain text
										if ui.metadataAreaClick.Clicked(gtx) {
											ui.showRichText = false
										}

										return ui.metadataAreaClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											// Render markdown using richtext
											spans := FormatMetadata(ui.markdownText, font.Typeface(""))
											if len(spans) == 0 {
												// Fallback to simple text if no spans generated
												label := material.Body2(ui.theme, ui.markdownText)
												label.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
												label.Font.Typeface = font.Typeface("monospace")
												label.TextSize = unit.Sp(20)
												return label.Layout(gtx)
											}
											textStyle := richtext.Text(&ui.metadataState, ui.theme.Shaper, spans...)
											return textStyle.Layout(gtx)
										})
									}))
								}

								return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
							} else {
								// Plain text mode - selectable with mouse
								// Only update text if content changed or mode switched
								if fullContent != ui.lastMetadataText ||
									ui.selectedIdx != ui.lastMetadataItemIdx ||
									ui.lastMetadataRichMode {
									ui.metadataEditor.SetText(fullContent)
									ui.lastMetadataText = fullContent
									ui.lastMetadataItemIdx = ui.selectedIdx
									ui.lastMetadataRichMode = false
								}

								editor := material.Editor(ui.theme, &ui.metadataEditor, "")
								editor.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
								editor.Font.Typeface = font.Typeface("monospace")
								editor.TextSize = unit.Sp(20)
								return editor.Layout(gtx)
							}
						}
						return layout.Dimensions{}
					}),
				)
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if ui.createMode {
				// Align editor to bottom of the pane
				return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Execute(key.FocusCmd{Tag: &ui.createEditor})
						editor := material.Editor(ui.theme, &ui.createEditor, "path/for/new/password")
						editor.TextSize = unit.Sp(18)
						// Add a border to the editor
						border := widget.Border{Color: color.NRGBA{A: 255}, CornerRadius: unit.Dp(4), Width: unit.Dp(2)}
						return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)
						})
					})
				})
			}
			if !ui.editMode && !ui.createMode {
				// Align button to bottom-right
				return layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(16)).Layout(gtx, ui.layoutAddButton)
				})
			}
			return layout.Dimensions{}
		}),
	)
}

func (ui *UI) layoutCountdown(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(100))
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: color.NRGBA{R: 102, G: 102, B: 102, A: 255}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (ui *UI) layoutProgressBar(gtx layout.Context) layout.Dimensions {
	barHeight := gtx.Dp(unit.Dp(4))
	fullWidth := gtx.Constraints.Max.X

	// Calculate filled width based on countdown progress
	ui.statusMutex.RLock()
	countdown := ui.countdown
	ui.statusMutex.RUnlock()

	filledWidth := min(max(int(float32(fullWidth)*countdown), 0), fullWidth)

	// Draw background (empty part)
	bgRect := image.Pt(fullWidth, barHeight)
	defer clip.Rect{Max: bgRect}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: color.NRGBA{R: 60, G: 60, B: 60, A: 255}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	// Draw filled part (progress)
	if filledWidth > 0 {
		filledRect := image.Pt(filledWidth, barHeight)
		defer clip.Rect{Max: filledRect}.Push(gtx.Ops).Pop()
		// Green-to-yellow-to-red gradient based on progress
		var progressColor color.NRGBA
		if countdown > 0.5 {
			// Green to yellow (1.0 -> 0.5)
			t := (countdown - 0.5) * 2
			progressColor = color.NRGBA{
				R: uint8(255 * (1 - t)),
				G: 200,
				B: 0,
				A: 255,
			}
		} else {
			// Yellow to red (0.5 -> 0.0)
			t := countdown * 2
			progressColor = color.NRGBA{
				R: 255,
				G: uint8(200 * t),
				B: 0,
				A: 255,
			}
		}
		paint.ColorOp{Color: progressColor}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}

	return layout.Dimensions{Size: image.Pt(fullWidth, barHeight)}
}

func (ui *UI) layoutKeyValueField(gtx layout.Context, pair KeyValuePair) layout.Dimensions {
	// Get or create field widget for this key
	fw, exists := ui.fieldWidgets[pair.Key]
	if !exists {
		fw = &fieldWidget{}
		fw.editor.ReadOnly = true
		fw.editor.SingleLine = true
		ui.fieldWidgets[pair.Key] = fw
	}

	// Update editor text if value changed
	if fw.editor.Text() != pair.Value {
		fw.editor.SetText(pair.Value)
	}

	// Handle clicks on label
	if fw.labelClickable.Clicked(gtx) {
		ui.copyFieldToClipboard(pair.Value)
	}

	// Handle clicks on input widget
	if fw.clickable.Clicked(gtx) {
		ui.copyFieldToClipboard(pair.Value)
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fw.labelClickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(ui.theme, pair.Key+":")
				label.Color = color.NRGBA{R: 238, G: 238, B: 238, A: 255}
				label.TextSize = unit.Sp(18)
				label.Font.Weight = font.Bold
				return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, label.Layout)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return fw.clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// First render the editor to get its dimensions
				macro := op.Record(gtx.Ops)
				editor := material.Editor(ui.theme, &fw.editor, "")
				editor.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
				editor.TextSize = unit.Sp(18)
				dims := editor.Layout(gtx)
				call := macro.Stop()

				// Draw rounded border using selection color
				borderColor := color.NRGBA{R: 100, G: 150, B: 200, A: 255} // Same as selection but fully opaque
				borderRadius := gtx.Dp(unit.Dp(4))
				borderWidth := gtx.Dp(unit.Dp(1))

				// Create outer rounded rectangle path for border
				outerRect := clip.RRect{
					Rect: image.Rectangle{Max: dims.Size},
					NW:   borderRadius, NE: borderRadius, SW: borderRadius, SE: borderRadius,
				}

				// Create inner rounded rectangle path for clipping
				innerRect := clip.RRect{
					Rect: image.Rectangle{
						Min: image.Pt(borderWidth, borderWidth),
						Max: image.Pt(dims.Size.X-borderWidth, dims.Size.Y-borderWidth),
					},
					NW: borderRadius - borderWidth, NE: borderRadius - borderWidth,
					SW: borderRadius - borderWidth, SE: borderRadius - borderWidth,
				}

				// Draw border by filling outer rect and subtracting inner rect
				defer outerRect.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: borderColor}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)

				// Subtract inner area to create border effect
				defer innerRect.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: color.NRGBA{A: 0}}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)

				// Draw the editor content on top
				call.Add(gtx.Ops)
				return dims
			})
		}),
	)
}

func (ui *UI) layoutPasswordField(gtx layout.Context, password string) layout.Dimensions {
	// Get or create field widget for password
	fw, exists := ui.fieldWidgets["password"]
	if !exists {
		fw = &fieldWidget{}
		fw.editor.ReadOnly = true
		fw.editor.SingleLine = true
		ui.fieldWidgets["password"] = fw
	}

	// Update editor text with masked password
	maskedValue := MaskPassword(password)
	if fw.editor.Text() != maskedValue {
		fw.editor.SetText(maskedValue)
	}

	// Handle clicks on label
	if fw.labelClickable.Clicked(gtx) {
		ui.copyFieldToClipboard(password)
	}

	// Handle clicks on input widget
	if fw.clickable.Clicked(gtx) {
		ui.copyFieldToClipboard(password)
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fw.labelClickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(ui.theme, "password:")
				label.Color = color.NRGBA{R: 238, G: 238, B: 238, A: 255}
				label.TextSize = unit.Sp(18)
				label.Font.Weight = font.Bold
				return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, label.Layout)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return fw.clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// First render the editor to get its dimensions
				macro := op.Record(gtx.Ops)
				editor := material.Editor(ui.theme, &fw.editor, "")
				editor.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
				editor.TextSize = unit.Sp(18)
				dims := editor.Layout(gtx)
				call := macro.Stop()

				// Draw rounded border using selection color
				borderColor := color.NRGBA{R: 100, G: 150, B: 200, A: 255}
				borderRadius := gtx.Dp(unit.Dp(4))
				borderWidth := gtx.Dp(unit.Dp(1))

				// Create outer rounded rectangle path for border
				outerRect := clip.RRect{
					Rect: image.Rectangle{Max: dims.Size},
					NW:   borderRadius, NE: borderRadius, SW: borderRadius, SE: borderRadius,
				}

				// Create inner rounded rectangle path for clipping
				innerRect := clip.RRect{
					Rect: image.Rectangle{
						Min: image.Pt(borderWidth, borderWidth),
						Max: image.Pt(dims.Size.X-borderWidth, dims.Size.Y-borderWidth),
					},
					NW: borderRadius - borderWidth, NE: borderRadius - borderWidth,
					SW: borderRadius - borderWidth, SE: borderRadius - borderWidth,
				}

				// Draw border by filling outer rect and subtracting inner rect
				defer outerRect.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: borderColor}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)

				// Subtract inner area to create border effect
				defer innerRect.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: color.NRGBA{A: 0}}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)

				// Draw the editor content on top
				call.Add(gtx.Ops)
				return dims
			})
		}),
	)
}

func (ui *UI) layoutAddButton(gtx layout.Context) layout.Dimensions {
	if ui.addButton.Clicked(gtx) {
		ui.createMode = true
		ui.createEditor.SetText("")
		gtx.Execute(key.FocusCmd{Tag: &ui.createEditor})
	}

	return ui.addButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Button dimensions
		size := gtx.Dp(unit.Dp(56))
		col := color.NRGBA{R: 100, G: 150, B: 200, A: 255} // light-blue

		// Draw circle
		bounds := image.Rect(0, 0, size, size)
		radius := float32(size) / 2.0

		defer clip.RRect{
			Rect: bounds,
			NW:   int(radius), NE: int(radius), SW: int(radius), SE: int(radius),
		}.Push(gtx.Ops).Pop()

		// Background color
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)

		// Draw '+' icon
		plusColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		plusThickness := float32(gtx.Dp(unit.Dp(3)))
		plusSize := float32(size) / 2.0
		center := float32(size) / 2.0

		// Horizontal bar
		hBar := image.Rect(
			int(center-plusSize/2), int(center-plusThickness/2),
			int(center+plusSize/2), int(center+plusThickness/2),
		)
		paint.FillShape(gtx.Ops, plusColor, clip.Rect(hBar).Op())

		// Vertical bar
		vBar := image.Rect(
			int(center-plusThickness/2), int(center-plusSize/2),
			int(center+plusThickness/2), int(center+plusSize/2),
		)
		paint.FillShape(gtx.Ops, plusColor, clip.Rect(vBar).Op())

		if ui.addButton.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}
