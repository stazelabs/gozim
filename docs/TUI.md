# zimtui - Terminal ZIM Browser

Console equivalent of zimserve: browse ZIM file content interactively in a terminal.

## Usage

```
zimtui <file.zim>              # open single ZIM
zimtui <directory>             # open all ZIMs in directory
zimtui *.zim                   # open multiple ZIMs
```

## Core Features

### Library View (multi-ZIM)
- Table listing loaded ZIM files: title, language, entry count, file size
- Filter/search across archives
- Enter to open an archive

### Article List View
- Scrollable list of C-namespace entries (content articles)
- Incremental title search / filter (uses `EntriesByTitlePrefix` - binary search, fast)
- Case-insensitive search toggle (warns: linear scan)
- Show redirect indicator on entries
- Sort toggle: by path vs by title

### Article View
- Display article content as rendered text (HTML -> plain text / styled terminal output)
- Metadata bar: title, MIME type, path, size
- Follow internal links (navigate to linked ZIM entries)
- Back/forward navigation stack

### Info Panel (toggle)
- Archive metadata: title, language, UUID, entry/cluster counts
- MIME type distribution
- Namespace breakdown
- Fulltext/title index availability
- Checksum verification (on demand)

## Open Questions

### HTML Rendering
The biggest challenge. ZIM content is primarily HTML. Options:

1. **Strip to plain text** - simplest, loses all formatting
2. **Terminal HTML renderer** (e.g., `glamour` for markdown-ish output, or a custom HTML-to-ANSI converter)
3. **Use `html2text` approach** - headings, lists, bold/italic via ANSI, skip images/CSS/JS
4. **Hybrid** - show rendered text with option to dump raw HTML

Recommendation: start with option 3 (styled plain text from HTML). Can iterate later.

### TUI Framework
Go TUI libraries to consider:

| Library | Pros | Cons |
|---------|------|------|
| **Bubble Tea** (`charmbracelet/bubbletea`) | Most popular, Elm architecture, rich ecosystem (lipgloss, bubbles) | Slightly more boilerplate |
| **tview** (`rivo/tview`) | Widget-based (tables, trees, forms), less code for standard layouts | Less flexible for custom views |
| **tcell** (`gdaber/tcell`) | Low-level, full control | Too much work for this scope |

Recommendation: **Bubble Tea** - largest community, best composability for the mix of views we need (list, viewport, search input). The `bubbles` library provides ready-made list, viewport, textinput, and table components.

### Content Types
ZIM files contain mixed content:
- HTML articles (primary) - need rendering
- Images (PNG, JPEG, SVG) - skip or show placeholder `[image: filename]`
- CSS/JS - skip (not useful in terminal)
- Metadata entries - show as key-value pairs

### Performance Considerations
- Large ZIMs (Wikipedia) can have millions of entries
- Entry listing must be lazy/paginated - iterators support this naturally
- Cluster cache size should be tunable (flag or config)
- Consider async content loading to keep UI responsive

### Link Navigation
- Parse `<a href="...">` from HTML content
- Map relative URLs to ZIM entry paths
- Maintain navigation history stack (back/forward)
- Highlight links in rendered output, allow tab-between and enter to follow

## Dependencies (not tied to core library)

```
github.com/charmbracelet/bubbletea    # TUI framework
github.com/charmbracelet/lipgloss     # Styling
github.com/charmbracelet/bubbles      # UI components (list, viewport, textinput)
github.com/spf13/cobra                # CLI (already used)
```

HTML rendering (evaluate):
```
github.com/JohannesKaufmann/html-to-markdown  # HTML -> markdown -> glamour
github.com/charmbracelet/glamour              # Markdown terminal renderer
```

Alternative: direct HTML-to-ANSI with a lightweight custom converter or `golang.org/x/net/html` tokenizer.

## Rough Architecture

```
cmd/zimtui/main.go
  - CLI setup (cobra), open archive(s)
  - Initialize Bubble Tea program

cmd/zimtui/model.go
  - Top-level model: current view state, archive handles
  - View enum: Library | ArticleList | Article | Info

cmd/zimtui/library.go    - Library list view (multi-ZIM)
cmd/zimtui/list.go        - Article list with search
cmd/zimtui/article.go     - Article content viewer
cmd/zimtui/info.go        - Archive info panel
cmd/zimtui/render.go      - HTML -> styled terminal text
cmd/zimtui/nav.go         - Navigation history stack
```

## Key Bindings (draft)

| Key | Action |
|-----|--------|
| `/` | Start search/filter |
| `Enter` | Open selected article |
| `Esc` / `q` | Back / quit |
| `Backspace` | Navigate back |
| `Tab` | Next link (in article view) |
| `i` | Toggle info panel |
| `?` | Help overlay |
| `j/k` or arrows | Navigate list |

## MVP Scope

A reasonable first pass:
1. Open single ZIM file
2. Article list with title search
3. Basic HTML-to-text article viewer (headings, bold, lists)
4. Back navigation
5. Archive info display

Defer to later:
- Multi-ZIM library view
- Link following within articles
- Image placeholders
- Configurable keybindings
- Mouse support
