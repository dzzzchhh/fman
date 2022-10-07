package model

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/76creates/stickers"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/nore-dev/fman/entry"
	"github.com/nore-dev/fman/theme"
)

type ListModel struct {
	entries []entry.Entry

	showHidden bool

	path string

	Width  int
	Height int

	selected_index int
	flexBox        *stickers.FlexBox

	maxEntryToShow int
	truncateLimit  int

	initialized bool

	lastClickedTime time.Time
	clickDelay      float64

	theme *theme.Theme

	lastKeyCharacter byte
}

type UpdateEntriesMsg struct {
	parent bool
}
type ClearKeyMsg struct {
}
type PathMsg struct {
	Path string
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func truncateText(str string, max int) string {
	// "hello world" -> "hello wo..."

	_str := str

	if len(str) > max {
		_str = str[:max-3] + "..."
	}

	return _str
}

const DIR_ICON = "📁"
const FILE_ICON = "📄"
const LINK_ICON = "🔗"

func assignListItemIcon(entry entry.Entry) string {
	if entry.IsSymLink() {
		return LINK_ICON
	} else if entry.IsDir {
		return DIR_ICON
	}
	return FILE_ICON
}

func detectOpenCommand() string {
	switch runtime.GOOS {
	case "linux":
		return "xdg-open"
	case "darwin":
		return "open"
	}

	return "start"
}

func changePath(path string) tea.Cmd {
	return func() tea.Msg {
		return PathMsg{Path: path}
	}
}

func updateEntry(newEntry entry.Entry) tea.Cmd {
	return func() tea.Msg {
		return entry.EntryMsg{Entry: newEntry}
	}
}

func sendMessage(message string) tea.Cmd {
	return func() tea.Msg {
		return NewMessageMsg{message}
	}
}

func NewListModel(theme *theme.Theme) ListModel {

	path, err := filepath.Abs(".")

	if err != nil {
		panic(err)
	}

	entries, err := entry.GetEntries(path, false)

	if err != nil {
		panic(err)
	}

	list := ListModel{
		path:          path,
		entries:       entries,
		truncateLimit: 100,
		flexBox:       stickers.NewFlexBox(0, 0),
		initialized:   false,
		clickDelay:    0.5,
		theme:         theme,
		showHidden:    false,
	}

	rows := []*stickers.FlexBoxRow{
		list.flexBox.NewRow().AddCells(
			[]*stickers.FlexBoxCell{
				stickers.NewFlexBoxCell(5, 1),
				stickers.NewFlexBoxCell(2, 1),
				stickers.NewFlexBoxCell(3, 1),
			},
		),
	}

	list.flexBox.AddRows(rows)

	return list
}

func (list ListModel) Init() tea.Cmd {
	return list.clearLastKey()
}

func (list ListModel) clearLastKey() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return ClearKeyMsg{}
	})
}

func (list *ListModel) getEntriesAbove() tea.Cmd {
	return changePath(filepath.Dir(list.path))
}

func (list *ListModel) getEntriesBelow() tea.Cmd {
	if !list.SelectedEntry().IsDir {
		return nil
	}

	path := filepath.Join(list.path, list.SelectedEntry().Name)

	if list.SelectedEntry().SymLinkPath != "" {
		path = list.SelectedEntry().SymLinkPath
	}

	return changePath(path)
}

func (list *ListModel) restrictIndex() {
	if list.selected_index < 0 {
		list.selected_index = len(list.entries) - 1
	} else if list.selected_index >= len(list.entries) {
		list.selected_index = 0
	}
}

func getFullPath(entry entry.Entry, path string) string {
	fullPath := filepath.Join(path, entry.Name)

	if entry.SymLinkPath != "" {
		fullPath = entry.SymLinkPath
	}

	return fullPath
}

func (list ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case PathMsg:
		var err error

		list.path = msg.Path
		list.entries, err = entry.GetEntries(list.path, list.showHidden)
		list.restrictIndex()

		// An error occured, give user a feedback
		if err != nil {
			return list, sendMessage(err.Error())
		}

		return list, updateEntry(list.SelectedEntry())
	case UpdateEntriesMsg:
		if msg.parent {
			return list, list.getEntriesAbove()
		}

		return list, list.getEntriesBelow()
	case ClearKeyMsg:
		list.lastKeyCharacter = ' '
		return list, list.clearLastKey()
	case tea.KeyMsg:
		switch msg.String() {
		case "m": // Show hidden files
			list.showHidden = !list.showHidden
			return list, func() tea.Msg {
				return PathMsg{list.path}
			}
		case "g": // Move to the beginning of the list
			if list.lastKeyCharacter == 'g' {
				list.selected_index = 0
			}
			list.lastKeyCharacter = 'g'
		case "G": // Move to the end of the list
			list.selected_index = len(list.entries) - 1

		case "~", ".": // Move to the home directory
			homeDir, _ := os.UserHomeDir()

			return list, changePath(homeDir)
		case "c": // Copy path to the clipboard
			path := getFullPath(list.SelectedEntry(), list.path)

			clipboard.WriteAll(path)

			return list, func() tea.Msg {
				return NewMessageMsg{"Copied!"}
			}
		case "w", "up", "k": // Select entry above
			list.selected_index -= 1
			list.restrictIndex()
			return list, updateEntry(list.SelectedEntry())
		case "s", "down", "j": // Select entry below
			list.selected_index += 1
			list.restrictIndex()

			return list, updateEntry(list.SelectedEntry())
		case "a", "left", "h": // Get entries from parent directory
			return list, func() tea.Msg {
				return UpdateEntriesMsg{parent: true}
			}
		case "d", "right", "l": // If the selected entry is a directory. Get entries under that directory
			return list, func() tea.Msg {
				return UpdateEntriesMsg{}
			}
		case "enter": // Open file with default application
			// Handle Symlink
			cmd := exec.Command(detectOpenCommand(), getFullPath(list.SelectedEntry(), list.path))
			cmd.Run()
		}

	case tea.WindowSizeMsg:
		list.flexBox.SetWidth(list.Width)
		list.flexBox.SetHeight(list.Height)

		list.flexBox.ForceRecalculate()

		list.truncateLimit = list.flexBox.Row(0).Cell(0).GetWidth() - 1
		list.maxEntryToShow = list.Height * 3 / 4

	case tea.MouseMsg:
		if msg.Type != tea.MouseLeft || !zone.Get("list").InBounds(msg) {
			return list, nil
		}

		_, y := zone.Get("list").Pos(msg)

		offset := 2

		if y < offset || y > len(list.entries)+offset-1 {
			return list, nil
		}

		list.selected_index = y + max(0, list.selected_index-list.maxEntryToShow) - offset

		// Double click
		time := time.Now()

		if time.Sub(list.lastClickedTime).Seconds() < list.clickDelay && list.SelectedEntry().IsDir {
			list.getEntriesBelow()
			list.restrictIndex()
			return list, func() tea.Msg {
				return UpdateEntriesMsg{}
			}
		}

		list.lastClickedTime = time
		// Update entry info model
		return list, func() tea.Msg {
			return entry.EntryMsg{Entry: list.SelectedEntry()}
		}

	}

	list.restrictIndex()

	if !list.initialized {
		list.initialized = true

		return list, tea.Batch(
			func() tea.Msg {
				return PathMsg{list.path}
			},
			func() tea.Msg {
				return entry.EntryMsg{Entry: list.SelectedEntry()}
			},
		)
	}

	return list, nil

}

func (list ListModel) View() string {
	list.flexBox.ForceRecalculate()

	if len(list.entries) == 0 {
		return "Empty"
	}

	cellsLength := list.flexBox.Row(0).CellsLen()
	contents := make([]strings.Builder, cellsLength)

	// Write List headers
	contents[0].WriteString(theme.BoldStyle.Render("Name"))
	contents[0].WriteByte('\n')

	contents[1].WriteString(theme.BoldStyle.Render("Size"))
	contents[1].WriteByte('\n')

	contents[2].WriteString(theme.BoldStyle.Render("Modify Time"))
	contents[2].WriteByte('\n')

	startIndex := max(0, list.selected_index-list.maxEntryToShow)
	stopIndex := min(len(list.entries), startIndex+list.maxEntryToShow+(list.Height*1/4))

	for index := startIndex; index < stopIndex; index++ {
		entry := list.entries[index]
		content := make([]strings.Builder, cellsLength)

		name := assignListItemIcon(entry) + " " + truncateText(entry.Name, list.truncateLimit)

		// TODO: also see https://github.com/nore-dev/fman/issues/2
		content[0].WriteString(name)
		content[1].WriteString(entry.Size)
		content[2].WriteString(entry.ModifyTime)

		var style lipgloss.Style
		for i := 0; i < cellsLength; i++ {

			offset := 0

			if index == list.selected_index {
				style = theme.SelectedItemStyle
			} else if index%2 == 0 {
				style = theme.EvenItemStyle
			}

			// IDK
			if i == 2 {
				offset = 2
			}

			style = style.Width(list.flexBox.Row(0).Cell(i).GetWidth() - offset)

			if i == 0 && entry.SymlinkName != "" {
				style = style.Bold(true).Underline(true)
			} else {
				style = style.UnsetBold().UnsetUnderline()
			}

			// Colors
			if index == list.selected_index {
				style = style.Foreground(list.Theme().SelectedItemFgColor)
			} else if entry.Name[0] == '.' {
				style = style.Foreground(list.Theme().HiddenFileColor)

				if entry.IsDir {
					style = style.Foreground(list.Theme().HiddenFolderColor)
				}
			} else if entry.IsDir {
				style = style.Foreground(list.Theme().FolderColor)
			} else {
				style = style.Foreground(list.Theme().TextColor)
			}

			if i != 0 && index != list.selected_index {
				style = style.Foreground(list.Theme().TextColor)
			}

			contents[i].WriteString(style.Render(content[i].String()))
			contents[i].WriteByte('\n')
		}
	}

	for i := 0; i < cellsLength; i++ {
		list.flexBox.Row(0).Cell(i).SetContent(contents[i].String())
	}

	return list.flexBox.Render()
}

func (list ListModel) SelectedEntry() entry.Entry {

	if len(list.entries) == 0 {
		return entry.Entry{}
	}

	return list.entries[list.selected_index]
}

func (list ListModel) Theme() *theme.Theme {
	return list.theme
}
