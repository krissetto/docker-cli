package container

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/pflag"
)

// Msg type for representing an error
type errMsg error

type runParamType struct {
	name          string
	splitOn       string
	editableIndex int
}

var (
	volumeParam = runParamType{
		name:          "--volume",
		splitOn:       ":",
		editableIndex: 0,
	}
	nameParam = runParamType{
		name: "--name",
	}
	entrypointParam = runParamType{
		name: "--entrypoint",
	}
	envVarParam = runParamType{
		name:          "--env",
		splitOn:       "=",
		editableIndex: 1,
	}
	portParam = runParamType{
		name:          "-p",
		splitOn:       ":",
		editableIndex: 0,
	}
)

type runParam struct {
	paramType    runParamType
	value        string
	valueOptions []string
}

type runFlags string

const (
	interactiveFlag runFlags = "--interactive"
	ttyFlag         runFlags = "--tty"
	detachFlag      runFlags = "--detach"
)

type runTUIModel struct {
	// values from standard run command
	ctx   context.Context
	flags *pflag.FlagSet
	ropts *runOptions
	copts *containerOptions

	// values used for the TUI
	imageName         string
	runParams         []runParam
	runFlags          []runFlags
	selectedParameter int
	editingParameter  bool
	parameterParts    []string
	editValue         string
	cursorPosition    int
	err               error
}

func (m runTUIModel) Init() tea.Cmd {
	return nil
}

func (m runTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case errMsg:
		// Store the error in the model
		m.err = msg
		return m, tea.Quit
	case tea.KeyMsg:
		if m.editingParameter {
			switch msg.String() {
			case "enter", "tab":
				currentParam := &m.runParams[m.selectedParameter]
				if currentParam.paramType.splitOn != "" {
					// if the parameter is splittable, then we need to edit only the index of that split that is editable
					m.parameterParts[currentParam.paramType.editableIndex] = m.editValue
					currentParam.value = strings.Join(m.parameterParts, currentParam.paramType.splitOn)
				} else {
					currentParam.value = m.editValue
				}
				// If the value is empty after editing, set it to the first option
				if currentParam.value == "" && len(currentParam.valueOptions) > 0 {
					currentParam.value = currentParam.valueOptions[0]
				}
				m.editingParameter = false
				m.editValue = ""
				m.parameterParts = nil
				m.cursorPosition = 0
				if msg.String() == "tab" {
					// Move to the next parameter
					if m.selectedParameter < len(m.runParams)-1 {
						m.selectedParameter++
					} else {
						m.selectedParameter = 0 // Wrap around to the first parameter
					}
				}
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
			case "ctrl+c", "esc":
				return m, tea.Quit
			case "enter": // TODO(krissetto): clean this up
				return m, tea.Quit
			case "left", "up", "shift+tab":
				if m.selectedParameter > 0 {
					m.selectedParameter--
				} else {
					m.selectedParameter = len(m.runParams) - 1 // Wrap around to the last parameter
				}
			case "right", "down", "tab":
				if m.selectedParameter < len(m.runParams)-1 {
					m.selectedParameter++
				} else {
					m.selectedParameter = 0 // Wrap around to the first parameter
				}
			default:
				// Enter editing mode if an alphanumeric key is pressed
				if len(msg.String()) == 1 && (msg.String()[0] >= 'a' && msg.String()[0] <= 'z' ||
					msg.String()[0] >= 'A' && msg.String()[0] <= 'Z' ||
					msg.String()[0] >= '0' && msg.String()[0] <= '9') {
					m.editingParameter = true
					currentParam := m.runParams[m.selectedParameter]

					// set default value if not present
					if currentParam.value == "" {
						currentParam.value = currentParam.valueOptions[0]
					}
					// if the parameter is splittable, then we need to edit only the index of that split that is editable
					if currentParam.paramType.splitOn != "" {
						// save the split out parts of the parameter string
						m.parameterParts = strings.Split(currentParam.value, currentParam.paramType.splitOn)
					}
					m.editValue += msg.String()
					m.cursorPosition = len(m.editValue)
				}
			}
		}
	}
	return m, nil
}

func (m runTUIModel) View() string {
	blueColor := "\033[94m"
	yellowColor := "\033[33;2m"
	resetColor := "\033[0m"
	grayColor := "\033[90m"
	cursorColor := "\033[7m"
	redColor := "\033[31m"

	if m.err != nil {
		return fmt.Sprintf("%s%v\n", redColor, m.err)
	}

	var params []string

	for i, param := range m.runParams {
		var paramStr string
		displayValue := param.value
		if displayValue == "" && len(param.valueOptions) > 0 {
			displayValue = param.valueOptions[0]
		}

		isEdited := param.value != "" && param.value != param.valueOptions[0]

		if i == m.selectedParameter {
			if m.editingParameter {
				beforeCursor := m.editValue[:m.cursorPosition]
				afterCursor := m.editValue[m.cursorPosition:]
				cursorChar := " "

				beforeEditedPart := ""
				afterEditedPart := ""

				if m.cursorPosition < len(m.editValue) {
					cursorChar = string(m.editValue[m.cursorPosition])
					afterCursor = m.editValue[m.cursorPosition+1:]
				}
				if m.parameterParts != nil {
					beforeEditedPart = strings.Join(
						m.parameterParts[:m.runParams[m.selectedParameter].paramType.editableIndex],
						param.paramType.splitOn,
					)
					if m.runParams[m.selectedParameter].paramType.editableIndex+1 < len(m.parameterParts) {
						afterEditedPart = param.paramType.splitOn + strings.Join(
							m.parameterParts[m.runParams[m.selectedParameter].paramType.editableIndex+1:],
							param.paramType.splitOn)
					}
				}
				// lovely string formatting trash
				paramStr = fmt.Sprintf("%s %s%s%s%s%s%s%s%s%s%s%s",
					param.paramType.name,
					beforeEditedPart,
					blueColor, beforeCursor,
					cursorColor, cursorChar,
					blueColor, afterCursor, resetColor,
					grayColor, afterEditedPart,
					resetColor)
			} else {
				paramStr = fmt.Sprintf("%s%s %s%s", blueColor, param.paramType.name, displayValue, resetColor)
			}
		} else if isEdited {
			paramStr = fmt.Sprintf("%s%s %s%s", yellowColor, param.paramType.name, displayValue, resetColor)
		} else {
			paramStr = fmt.Sprintf("%s %s", param.paramType.name, displayValue)
		}
		params = append(params, paramStr)
	}

	flagStrings := make([]string, len(m.runFlags))
	for i, flag := range m.runFlags {
		flagStrings[i] = fmt.Sprintf("%s%s%s", grayColor, string(flag), resetColor)
	}

	// Construct the command string
	commandParts := []string{fmt.Sprintf("%sdocker run%s", grayColor, resetColor)}
	commandParts = append(commandParts, flagStrings...)
	commandParts = append(commandParts, params...)
	commandParts = append(commandParts, fmt.Sprintf("%s%s%s", grayColor, m.imageName, resetColor))

	// Join command parts with line breaks and backslashes if too long
	command := strings.Join(commandParts, " ")
	if len(command) > 80 {
		command = strings.Join(commandParts, fmt.Sprintf("%s \\\n%s    ", grayColor, resetColor))
	}

	footerLegend := "\n\n"
	if m.editingParameter {
		footerLegend += fmt.Sprintf("%sEnter: Confirm | Tab: Confirm and Next | Esc: Cancel%s", grayColor, resetColor)
	} else {
		footerLegend += fmt.Sprintf("%s↑/↓/←/→: Navigate | Tab: Next | Type: Edit | Enter: Execute | Esc: Quit%s", grayColor, resetColor)
	}

	return command + footerLegend
}

// getParamOptions returns the options for the parameters that can be edited for the given image name
// TODO(krissetto): these are the options that we need to get dynamically given the image name
// The values here need to be changed to be more in line with what each image might need as sane defaults
func getParamOptions(imageName string) []runParam {
	imageParamMap := map[string][]runParam{
		"alpine": {
			{
				paramType:    nameParam,
				valueOptions: []string{"alpine-test", "evenCoolerName"},
			},
			{
				paramType:    portParam,
				valueOptions: []string{"8080:80"},
			},
			{
				paramType:    entrypointParam,
				valueOptions: []string{"/bin/ash"},
			},
		},
		"postgres": {
			{
				paramType:    nameParam,
				valueOptions: []string{"postgresDB", "evenCoolerName"},
			},
			{
				paramType:    portParam,
				valueOptions: []string{"5432:5432"},
			},
			{
				paramType:    volumeParam,
				valueOptions: []string{"postgres-data:/some/other/container/dir", "/yet/another/local/dir:/yet/another/container/dir"},
			},
			{
				paramType:    envVarParam,
				valueOptions: []string{"POSTGRES_USER=test-user"},
			},
			{
				paramType:    envVarParam,
				valueOptions: []string{"POSTGRES_PASSWORD=test-password"},
			},
			{
				paramType:    envVarParam,
				valueOptions: []string{"POSTGRES_DB=test-db"},
			},
		},
	}
	return imageParamMap[imageName]
}

func getFlags(imageName string) []runFlags {
	imageFlagMap := map[string][]runFlags{
		"alpine": {
			interactiveFlag,
			ttyFlag,
		},
		"postgres": {
			detachFlag,
		},
	}
	return imageFlagMap[imageName]
}

func runTUI(ctx context.Context, flags *pflag.FlagSet, ropts *runOptions, copts *containerOptions) (*pflag.FlagSet, *runOptions, *containerOptions, error) {
	// run bubbletea tui here
	tuiModel := initialModel(ctx, flags, ropts, copts)
	tui := tea.NewProgram(tuiModel, tea.WithContext(context.WithoutCancel(ctx)))

	finalModel, err := tui.Run()
	if err != nil {
		return nil, nil, nil, err
	}

	// Convert the final model back to runTUIModel
	finalRunTUIModel, ok := finalModel.(runTUIModel)
	if !ok {
		return nil, nil, nil, fmt.Errorf("unexpected model type")
	}

	// Check if the TUI was exited without an error
	if finalRunTUIModel.err == nil {
		// Apply the changes made in the TUI to the actual options
		for _, param := range finalRunTUIModel.runParams {
			switch param.paramType {
			// TODO(krissetto): Add more cases for other parameters as needed
			case nameParam:
				finalRunTUIModel.ropts.name = param.value
			case volumeParam:
				finalRunTUIModel.copts.volumes.Set(param.value)
			case entrypointParam:
				finalRunTUIModel.copts.entrypoint = param.value
			case envVarParam:
				finalRunTUIModel.copts.env.Set(param.value)
			case portParam:
				finalRunTUIModel.copts.publish.Set(param.value)
			}
		}
		for _, flag := range finalRunTUIModel.runFlags {
			switch flag {
			case detachFlag:
				finalRunTUIModel.ropts.detach = true
			case interactiveFlag:
				finalRunTUIModel.copts.stdin = true
			case ttyFlag:
				finalRunTUIModel.copts.tty = true
			}
			finalRunTUIModel.flags.Set(string(flag), "true")
		}
	}

	// return values from model
	return finalRunTUIModel.flags, finalRunTUIModel.ropts, finalRunTUIModel.copts, finalRunTUIModel.err
}

func initialModel(ctx context.Context, flags *pflag.FlagSet, ropts *runOptions, copts *containerOptions) runTUIModel {
	runParams := getParamOptions(copts.Image)

	// Set initial values for runParams
	for i := range runParams {
		if runParams[i].value == "" && len(runParams[i].valueOptions) > 0 {
			runParams[i].value = runParams[i].valueOptions[0]
		}
	}

	return runTUIModel{
		// docker cli values
		ctx:   ctx,
		flags: flags,
		ropts: ropts,
		copts: copts,

		// tui values
		imageName:         copts.Image,
		runParams:         runParams,
		runFlags:          getFlags(copts.Image),
		selectedParameter: 0,
		editingParameter:  false,
		editValue:         "",
		cursorPosition:    0,
	}
}
