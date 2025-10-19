package ui

import (
	"bytes"
	"image/color"
	"regexp"
	"strings"

	"gioui.org/font"
	"gioui.org/unit"
	"gioui.org/x/richtext"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	goldmarktext "github.com/yuin/goldmark/text"
)

// Color scheme for formatted text
var (
	keyColor           = color.NRGBA{R: 238, G: 238, B: 238, A: 255} // #EEE
	valueColor         = color.NRGBA{R: 200, G: 200, B: 200, A: 255} // #C8C8C8
	headingColor       = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // #FFF
	textColor          = color.NRGBA{R: 200, G: 200, B: 200, A: 255} // #C8C8C8
	codeColor          = color.NRGBA{R: 180, G: 180, B: 180, A: 255} // Slightly darker
	linkColor          = color.NRGBA{R: 107, G: 164, B: 231, A: 255} // #6BA4E7
	blockquoteColor    = color.NRGBA{R: 160, G: 160, B: 160, A: 255} // #A0A0A0
	strikethroughColor = color.NRGBA{R: 140, G: 140, B: 140, A: 255} // #8C8C8C (darker gray)
	tableHeaderColor   = color.NRGBA{R: 220, G: 220, B: 220, A: 255} // #DCDCDC
	tableBorderColor   = color.NRGBA{R: 150, G: 150, B: 150, A: 255} // #969696
)

// Key-value pattern: word(s) without spaces or colons, followed by colon
var keyValuePattern = regexp.MustCompile(`^([^\s:]+):\s*(.*)$`)

// KeyValuePair represents a single key-value field
type KeyValuePair struct {
	Key   string
	Value string
}

// MaskPassword returns a masked representation of a password
func MaskPassword(password string) string {
	if password == "" {
		return "<empty>"
	}
	return "***<has value>***"
}

// ExtractKeyValuePairs parses text and separates key:value pairs from markdown content.
// Returns the array of key-value pairs and remaining text (markdown/other content).
func ExtractKeyValuePairs(text string) ([]KeyValuePair, string) {
	if text == "" {
		return nil, ""
	}

	lines := strings.Split(text, "\n")
	var pairs []KeyValuePair
	var remainingLines []string
	inKeyValueSection := true

	for _, line := range lines {
		if !inKeyValueSection {
			remainingLines = append(remainingLines, line)
			continue
		}

		// Check for key:value pattern
		if matches := keyValuePattern.FindStringSubmatch(line); matches != nil {
			pairs = append(pairs, KeyValuePair{
				Key:   matches[1],
				Value: matches[2],
			})
			continue
		}

		// Check for markdown start (heading)
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			inKeyValueSection = false
			remainingLines = append(remainingLines, line)
			continue
		}

		// Empty line - stay in key-value section, don't add to pairs
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Non-key-value line - switch to markdown mode
		inKeyValueSection = false
		remainingLines = append(remainingLines, line)
	}

	remainingText := strings.Join(remainingLines, "\n")
	return pairs, strings.TrimSpace(remainingText)
}

// FormatMetadata parses text and returns formatted spans for richtext rendering.
// It handles key:value pairs (with bold keys and prefix) and markdown sections.
func FormatMetadata(text string, shaper font.Typeface) []richtext.SpanStyle {
	if text == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	var spans []richtext.SpanStyle
	inMarkdownMode := false
	var markdownBuffer strings.Builder

	for i, line := range lines {
		// If we're in markdown mode, collect all remaining text
		if inMarkdownMode {
			if markdownBuffer.Len() > 0 {
				markdownBuffer.WriteString("\n")
			}
			markdownBuffer.WriteString(line)
			continue
		}

		// Check for markdown start (heading or non-key-value line)
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			// Switch to markdown mode
			inMarkdownMode = true
			// Collect remaining text starting from this line
			markdownBuffer.WriteString(line)
			continue
		}

		// Check for key:value pattern
		if matches := keyValuePattern.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			value := matches[2]

			// Add prefix with key in bold
			spans = append(spans, richtext.SpanStyle{
				Content: "▸ " + key + ":",
				Color:   keyColor,
				Size:    unit.Sp(20),
				Font: font.Font{
					Typeface: shaper,
					Weight:   font.Bold,
				},
			})

			// Add value in normal weight
			if value != "" {
				spans = append(spans, richtext.SpanStyle{
					Content: " " + value,
					Color:   valueColor,
					Size:    unit.Sp(20),
					Font: font.Font{
						Typeface: shaper,
						Weight:   font.Normal,
					},
				})
			}

			// Add newline unless it's the last line
			if i < len(lines)-1 {
				spans = append(spans, richtext.SpanStyle{
					Content: "\n",
					Color:   textColor,
					Size:    unit.Sp(20),
					Font: font.Font{
						Typeface: shaper,
						Weight:   font.Normal,
					},
				})
			}
			continue
		}

		// Empty line - add newline and stay in key-value mode
		if strings.TrimSpace(line) == "" {
			spans = append(spans, richtext.SpanStyle{
				Content: "\n",
				Color:   textColor,
				Size:    unit.Sp(20),
				Font: font.Font{
					Typeface: shaper,
					Weight:   font.Normal,
				},
			})
			continue
		}

		// Non-key-value line - switch to markdown mode
		inMarkdownMode = true
		markdownBuffer.WriteString(line)
	}

	// Process markdown if we collected any
	if markdownBuffer.Len() > 0 {
		markdownSpans := parseMarkdown(markdownBuffer.String(), shaper)
		spans = append(spans, markdownSpans...)
	}

	return spans
}

// parseMarkdown parses markdown text and converts it to richtext spans
func parseMarkdown(text string, shaper font.Typeface) []richtext.SpanStyle {
	var spans []richtext.SpanStyle

	// Create goldmark parser with Table and Strikethrough extensions
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			extension.Strikethrough,
		),
	)

	reader := goldmarktext.NewReader([]byte(text))
	doc := md.Parser().Parse(reader)

	// Walk the AST and convert to spans
	context := &markdownContext{
		shaper: shaper,
		spans:  &spans,
		source: []byte(text),
	}

	ast.Walk(doc, context.visitor)

	return spans
}

// markdownContext holds state while walking the markdown AST
type markdownContext struct {
	shaper          font.Typeface
	spans           *[]richtext.SpanStyle
	source          []byte
	listDepth       int
	listCounters    []int // For ordered lists
	inEmphasis      bool
	inStrong        bool
	inCode          bool
	inBlockquote    bool
	inHeading       int // Heading level (0 = not in heading)
	inStrikethrough bool
	inTable         bool
	inTableHeader   bool
	tableColumnIdx  int
}

// visitor walks the markdown AST and builds richtext spans
func (ctx *markdownContext) visitor(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch node := n.(type) {
	case *ast.Document:
		// Nothing special for document

	case *ast.Heading:
		if entering {
			ctx.inHeading = node.Level
		} else {
			ctx.inHeading = 0
			// Add newline after heading
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: "\n",
				Color:   textColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		}

	case *ast.Paragraph:
		if !entering && n.NextSibling() != nil {
			// Add newline after paragraph unless it's the last element
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: "\n",
				Color:   textColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		}

	case *ast.List:
		if entering {
			ctx.listDepth++
			if node.IsOrdered() {
				ctx.listCounters = append(ctx.listCounters, node.Start)
			} else {
				ctx.listCounters = append(ctx.listCounters, 0)
			}
		} else {
			ctx.listDepth--
			if len(ctx.listCounters) > 0 {
				ctx.listCounters = ctx.listCounters[:len(ctx.listCounters)-1]
			}
			// Add newline after list
			if n.NextSibling() != nil {
				*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
					Content: "\n",
					Color:   textColor,
					Size:    unit.Sp(20),
					Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
				})
			}
		}

	case *ast.ListItem:
		if entering {
			indent := strings.Repeat("  ", ctx.listDepth-1)
			var marker string
			if len(ctx.listCounters) > 0 && ctx.listCounters[len(ctx.listCounters)-1] > 0 {
				// Ordered list
				marker = string(rune('0'+ctx.listCounters[len(ctx.listCounters)-1])) + ". "
				ctx.listCounters[len(ctx.listCounters)-1]++
			} else {
				// Unordered list
				marker = "• "
			}
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: indent + marker,
				Color:   textColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		} else {
			// Add newline after list item
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: "\n",
				Color:   textColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		}

	case *ast.Blockquote:
		if entering {
			ctx.inBlockquote = true
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: "│ ",
				Color:   blockquoteColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		} else {
			ctx.inBlockquote = false
		}

	case *ast.CodeBlock, *ast.FencedCodeBlock:
		if entering {
			var buf bytes.Buffer
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				buf.Write(line.Value(ctx.source))
			}
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: buf.String(),
				Color:   codeColor,
				Size:    unit.Sp(18),
				Font:    font.Font{Typeface: "monospace", Weight: font.Normal},
			})
			if n.NextSibling() != nil {
				*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
					Content: "\n",
					Color:   textColor,
					Size:    unit.Sp(20),
					Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
				})
			}
		}
		return ast.WalkSkipChildren, nil

	case *ast.Emphasis:
		if entering {
			// Level 1 = italic (*text*), Level 2 = bold (**text**)
			if node.Level == 2 {
				ctx.inStrong = true
			} else {
				ctx.inEmphasis = true
			}
		} else {
			if node.Level == 2 {
				ctx.inStrong = false
			} else {
				ctx.inEmphasis = false
			}
		}

	case *ast.CodeSpan:
		if entering {
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: string(node.Text(ctx.source)),
				Color:   codeColor,
				Size:    unit.Sp(18),
				Font:    font.Font{Typeface: "monospace", Weight: font.Normal},
			})
		}
		return ast.WalkSkipChildren, nil

	case *ast.Link:
		if entering {
			// For links, we'll show the link text in link color
			// The actual URL is in node.Destination
		}

	case *extast.Strikethrough:
		if entering {
			ctx.inStrikethrough = true
		} else {
			ctx.inStrikethrough = false
		}

	case *extast.Table:
		if entering {
			ctx.inTable = true
		} else {
			ctx.inTable = false
			// Add newline after table
			if n.NextSibling() != nil {
				*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
					Content: "\n",
					Color:   textColor,
					Size:    unit.Sp(20),
					Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
				})
			}
		}

	case *extast.TableHeader:
		if entering {
			ctx.inTableHeader = true
			ctx.tableColumnIdx = 0
		} else {
			ctx.inTableHeader = false
			// Add separator line after header
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: "\n",
				Color:   tableBorderColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		}

	case *extast.TableRow:
		if entering {
			ctx.tableColumnIdx = 0
		} else {
			// Add newline after row
			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: "\n",
				Color:   textColor,
				Size:    unit.Sp(20),
				Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
			})
		}

	case *extast.TableCell:
		if entering {
			if ctx.tableColumnIdx > 0 {
				// Add separator between cells
				*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
					Content: " │ ",
					Color:   tableBorderColor,
					Size:    unit.Sp(20),
					Font:    font.Font{Typeface: ctx.shaper, Weight: font.Normal},
				})
			}
			ctx.tableColumnIdx++
		}

	case *ast.Text:
		if entering {
			content := string(node.Segment.Value(ctx.source))

			// Determine font weight and style
			weight := font.Normal
			if ctx.inStrong || ctx.inHeading > 0 {
				weight = font.Bold
			}

			// Determine size based on heading level
			size := unit.Sp(20)
			if ctx.inHeading > 0 {
				switch ctx.inHeading {
				case 1:
					size = unit.Sp(32)
				case 2:
					size = unit.Sp(28)
				case 3:
					size = unit.Sp(24)
				default:
					size = unit.Sp(22)
				}
			}

			// Determine color
			col := textColor
			if ctx.inHeading > 0 {
				col = headingColor
			} else if ctx.inBlockquote {
				col = blockquoteColor
			} else if n.Parent() != nil && n.Parent().Kind() == ast.KindLink {
				col = linkColor
			} else if ctx.inStrikethrough {
				col = strikethroughColor
			} else if ctx.inTableHeader {
				col = tableHeaderColor
				weight = font.Bold
			}

			// Determine typeface
			typeface := ctx.shaper
			if ctx.inCode {
				typeface = "monospace"
			}

			// Handle line breaks in text
			if node.SoftLineBreak() {
				content += "\n"
			}

			*ctx.spans = append(*ctx.spans, richtext.SpanStyle{
				Content: content,
				Color:   col,
				Size:    size,
				Font: font.Font{
					Typeface: typeface,
					Weight:   weight,
					Style:    font.Regular,
				},
			})
		}
	}

	return ast.WalkContinue, nil
}
