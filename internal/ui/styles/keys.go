package styles

import "github.com/charmbracelet/bubbles/key"

type GlobalKeyMap struct {
	Quit key.Binding
	Next key.Binding
	Prev key.Binding
	Help key.Binding
}

var GlobalKeys = GlobalKeyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "종료"),
	),
	Next: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "다음"),
	),
	Prev: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "이전"),
	),
}

type ListKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Add    key.Binding
	Edit   key.Binding
	Delete key.Binding
	Toggle key.Binding
}

var ListKeys = ListKeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "위로")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "아래로")),
	Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "추가")),
	Edit:   key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/enter", "편집")),
	Delete: key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "삭제")),
	Toggle: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "토글")),
}

type FormKeyMap struct {
	NextField key.Binding
	PrevField key.Binding
	Confirm   key.Binding
	Cancel    key.Binding
}

var FormKeys = FormKeyMap{
	NextField: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "다음 필드")),
	PrevField: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "이전 필드")),
	Confirm:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "확인")),
	Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "취소")),
}
