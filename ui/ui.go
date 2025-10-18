package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"time"

	"0xADE/xpass/config"
	"0xADE/xpass/passcard"
	"0xADE/xpass/storage"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
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

	initialized          bool
	metadataState        richtext.InteractiveText
	metadataEditor       widget.Editor
	showRichText         bool
	metadataAreaClick    widget.Clickable
	toggleButton         widget.Clickable
	lastMetadataText     string
	lastMetadataItemIdx  int
	lastMetadataRichMode bool
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
	}

	ui.searchEditor.SingleLine = true
	ui.searchEditor.Submit = true

	ui.metadataEditor.ReadOnly = true
	ui.metadataEditor.SingleLine = false

	store.Subscribe(func(status string) {
		ui.status = status
		ui.updateQuery()
		if ui.window != nil {
			ui.window.Invalidate()
		}
	})

	ui.updateQuery()
	return ui
}

func (ui *UI) updateQuery() {
	ui.filtered = ui.storage.Query(ui.query)
	if ui.selectedIdx >= len(ui.filtered) {
		ui.selectedIdx = 0
	}
}

func (ui *UI) copyToClipboard() {
	if ui.selectedIdx >= len(ui.filtered) {
		ui.status = "No password selected"
		return
	}

	pw := ui.filtered[ui.selectedIdx]
	pass := pw.Password()
	if err := clipboard.WriteAll(pass); err != nil {
		ui.status = fmt.Sprintf("Failed to copy: %v", err)
		return
	}

	ui.status = "Copied to clipboard"
	go ui.clearClipboard()
}

func (ui *UI) clearClipboard() {
	if ui.countingDown {
		ui.countdownDone <- true
	}
	ui.countingDown = true

	tick := 200 * time.Millisecond
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	remaining := float64(ui.config.PasswordStoreClipSeconds)
	for {
		select {
		case <-ui.countdownDone:
			ui.countingDown = false
			return
		case <-ticker.C:
			ui.countdown = float32(remaining / float64(ui.config.PasswordStoreClipSeconds))
			ui.status = fmt.Sprintf("Will clear %s in %.0f seconds", ui.storage.NameByIdx(ui.selectedIdx), remaining)
			if ui.window != nil {
				ui.window.Invalidate()
			}
			remaining -= tick.Seconds()
			if remaining <= 0 {
				clipboard.WriteAll("")
				ui.status = "Clipboard cleared"
				ui.countingDown = false
				if ui.window != nil {
					ui.window.Invalidate()
				}
				return
			}
		}
	}
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
			return e.Err

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			if !ui.initialized {
				gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
				ui.initialized = true
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
			for {
				ev, ok := gtx.Event(
					key.Filter{Name: key.NameEscape},
					key.Filter{Name: key.NameUpArrow},
					key.Filter{Name: key.NameDownArrow},
					key.Filter{Name: "T"},
				)
				if !ok {
					break
				}
				if kev, ok := ev.(key.Event); ok && kev.State == key.Press {
					switch kev.Name {
					case key.NameEscape:
						os.Exit(0)
					case key.NameUpArrow:
						if ui.selectedIdx > 0 {
							ui.selectedIdx--
							if ui.list.Position.First > ui.selectedIdx {
								ui.list.Position.First = ui.selectedIdx
							}
						}
					case key.NameDownArrow:
						if ui.selectedIdx < len(ui.filtered)-1 {
							ui.selectedIdx++
							if ui.list.Position.Count > 0 && ui.list.Position.First+ui.list.Position.Count <= ui.selectedIdx {
								ui.list.Position.First = ui.selectedIdx - ui.list.Position.Count + 1
							}
						}
					case "T":
						if kev.Modifiers.Contain(key.ModCtrl) {
							ui.showRichText = !ui.showRichText
						}
					}
				}
			}
			// ===== END OF KEYBOARD HANDLING =====

			for {
				ev, ok := ui.searchEditor.Update(gtx)
				if !ok {
					break
				}
				switch ev.(type) {
				case widget.ChangeEvent:
					ui.query = ui.searchEditor.Text()
					ui.updateQuery()
				case widget.SubmitEvent:
					ui.copyToClipboard()
				}
			}

			ui.layout(gtx)
			area.Pop()
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, ui.layoutLeftPane),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(ui.layoutRightPane),
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
				label := material.Body2(ui.theme, ui.status)
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

	buttonText := "📝 Show Plain Text"
	if !ui.showRichText {
		buttonText = "🎨 Show Rich Text"
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

func (ui *UI) layoutRightPane(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Max.X = gtx.Dp(unit.Dp(600))
	gtx.Constraints.Min.X = gtx.Dp(unit.Dp(300))

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			rect := clip.Rect{Max: gtx.Constraints.Max}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 68, G: 68, B: 68, A: 255}, rect.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ui.countingDown {
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
						return ui.layoutToggleButton(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if ui.selectedIdx < len(ui.filtered) {
							metadata := ui.filtered[ui.selectedIdx].Metadata()
							if metadata == "" {
								metadata = "Press Enter to decrypt"
							}

							// Handle clicks in rich text mode
							if ui.showRichText {
								// Mark that we're in rich text mode and track metadata
								if ui.selectedIdx != ui.lastMetadataItemIdx {
									ui.lastMetadataItemIdx = ui.selectedIdx
									ui.lastMetadataText = metadata
								}
								ui.lastMetadataRichMode = true

								// Check for click to switch to plain text
								if ui.metadataAreaClick.Clicked(gtx) {
									ui.showRichText = false
								}

								return ui.metadataAreaClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									// Rich text mode with formatting
									spans := FormatMetadata(metadata, font.Typeface(""))
									if len(spans) == 0 {
										// Fallback to simple text if no spans generated
										label := material.Body2(ui.theme, metadata)
										label.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
										label.Font.Typeface = font.Typeface("monospace")
										label.TextSize = unit.Sp(20)
										return label.Layout(gtx)
									}

									// Render using richtext
									textStyle := richtext.Text(&ui.metadataState, ui.theme.Shaper, spans...)
									return textStyle.Layout(gtx)
								})
							} else {
								// Plain text mode - selectable with mouse
								// Only update text if metadata changed or mode switched
								if metadata != ui.lastMetadataText ||
									ui.selectedIdx != ui.lastMetadataItemIdx ||
									ui.lastMetadataRichMode {
									ui.metadataEditor.SetText(metadata)
									ui.lastMetadataText = metadata
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
