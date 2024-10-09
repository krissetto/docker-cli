package container

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type runParam struct {
	name         string
	value        string
	valueOptions []string
}

type runModel struct {
	imageName         string
	runParams         []runParam
	selectedParameter int
	editingParameter  bool
	editValue         string
	cursorPosition    int // Add this line
}

func initialModel() runModel {
	return runModel{
		imageName: "alpine",
		runParams: []runParam{
			{name: "--name", value: "myName", valueOptions: []string{"someOtherName", "evenCoolerName"}},
			{name: "--volume", value: "/some/local/dir:/some/container/dir", valueOptions: []string{"/some/other/≈local/dir:/some/other/container/dir", "/yet/another/local/dir:/yet/another/container/dir"}},
		},
		selectedParameter: 0,
		editingParameter:  false,
		editValue:         "",
		cursorPosition:    0, // Add this line
	}
}

func (m runModel) Init() tea.Cmd {
	return nil
}

func (m runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editingParameter {
			switch msg.String() {
			case "enter":
				m.runParams[m.selectedParameter].value = m.editValue
				m.editingParameter = false
				m.editValue = ""
				m.cursorPosition = 0
			case "esc":
				m.editingParameter = false
				m.editValue = ""
				m.cursorPosition = 0
			case "backspace":
				if m.cursorPosition > 0 {
					m.editValue = m.editValue[:m.cursorPosition-1] + m.editValue[m.cursorPosition:]
					m.cursorPosition--
				}
			case "left":
				if m.cursorPosition > 0 {
					m.cursorPosition--
				}
			case "right":
				if m.cursorPosition < len(m.editValue) {
					m.cursorPosition++
				}
			default:
				if len(msg.String()) == 1 {
					m.editValue = m.editValue[:m.cursorPosition] + msg.String() + m.editValue[m.cursorPosition:]
					m.cursorPosition++
				}
			}
		} else {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "left":
				if m.selectedParameter > 0 {
					m.selectedParameter--
				}
			case "down", "right":
				if m.selectedParameter < len(m.runParams)-1 {
					m.selectedParameter++
				}
			case "enter":
				m.editingParameter = true
				m.editValue = m.runParams[m.selectedParameter].value
				m.cursorPosition = len(m.editValue) // Set cursor to end of value
			}
		}
	}
	return m, nil
}

func (m runModel) View() string {
	blueColor := "\033[34m"
	resetColor := "\033[0m"
	grayColor := "\033[90m"
	cursorColor := "\033[7m" // Inverse video for cursor

	var params []string
	for i, param := range m.runParams {
		paramStr := fmt.Sprintf("%s %s", param.name, param.value)
		if i == m.selectedParameter {
			if m.editingParameter {
				beforeCursor := m.editValue[:m.cursorPosition]
				afterCursor := m.editValue[m.cursorPosition:]
				cursorChar := " "
				if m.cursorPosition < len(m.editValue) {
					cursorChar = string(m.editValue[m.cursorPosition])
					afterCursor = m.editValue[m.cursorPosition+1:]
				}
				paramStr = fmt.Sprintf("%s%s %s%s%s%s%s%s%s",
					blueColor, param.name,
					beforeCursor,
					cursorColor, cursorChar, resetColor, blueColor,
					afterCursor, resetColor)
			} else {
				paramStr = fmt.Sprintf("%s%s%s", blueColor, paramStr, resetColor)
			}
		}
		params = append(params, paramStr)
	}

	command := fmt.Sprintf("docker run %s %s", strings.Join(params, " "), m.imageName)

	var legend string
	if m.editingParameter {
		legend = fmt.Sprintf("\n\n%sEnter: Confirm | Esc: Cancel%s", grayColor, resetColor)
	} else {
		legend = fmt.Sprintf("\n\n%s←/→: Navigate parameters | q: Quit | Enter: Edit parameter%s", grayColor, resetColor)
	}

	return command + legend
}

func runTUI() error {
	// run bubbletea app here
	app := tea.NewProgram(initialModel())
	_, err := app.Run()
	return err
}
