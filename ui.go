package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ollama/ollama/api"
)

var (
	// Gruvbox Dark Palette
	gbOrange    = lipgloss.Color("#fe8019")
	gbYellow    = lipgloss.Color("#fabd2f")
	gbGreen     = lipgloss.Color("#b8bb26")
	gbRed       = lipgloss.Color("#fb4934")
	gbBlue      = lipgloss.Color("#83a598")
	gbDarkGray  = lipgloss.Color("#504945")
	gbLightGray = lipgloss.Color("#a89984")

	docStyle        = lipgloss.NewStyle().Margin(0, 0)
	titleStyle      = lipgloss.NewStyle().Foreground(gbYellow).Bold(true).MarginBottom(1)
	paneStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gbDarkGray).Foreground(gbLightGray)
	activePaneStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gbOrange).Foreground(gbLightGray)
	statusStyle     = lipgloss.NewStyle().Foreground(gbLightGray)
	successLogStyle = lipgloss.NewStyle().Foreground(gbGreen)
	errorLogStyle   = lipgloss.NewStyle().Foreground(gbRed)
	infoLogStyle    = lipgloss.NewStyle().Foreground(gbBlue)
	progressStyle   = lipgloss.NewStyle().Foreground(gbOrange)
)

type modelItem struct {
	model api.ListModelResponse
}

func (i modelItem) Title() string { return i.model.Name }
func (i modelItem) Description() string {
	size := float64(i.model.Size) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.2f GB", size)
}
func (i modelItem) FilterValue() string { return i.model.Name }

type uiState int

const (
	stateBrowsing uiState = iota
	statePulling
	stateChat
	stateOllamaNotRunning
	stateOllamaNotInstalled
	stateOpenWith
	stateRegistry
)

type errMsg error

type pullResponse struct {
	progress api.ProgressResponse
	err      error
	done     bool
}

type pullProgressMsg pullResponse

func waitForProgress(sub chan pullResponse) tea.Cmd {
	return func() tea.Msg {
		if sub == nil {
			return nil
		}
		resp, ok := <-sub
		if !ok {
			return nil
		}
		return pullProgressMsg(resp)
	}
}

type pullCompleteMsg struct{}

type loadModelsMsg struct {
	models []api.ListModelResponse
}

type logMsg struct {
	text  string
	level string
}

type appItem struct {
	name string
	cmd  string
}

func (i appItem) Title() string       { return i.name }
func (i appItem) Description() string { return i.cmd }
func (i appItem) FilterValue() string { return i.name }

type registryItem struct {
	tag string
}

func (i registryItem) Title() string       { return i.tag }
func (i registryItem) Description() string { return "Remote model available for pull" }
func (i registryItem) FilterValue() string { return i.tag }

type registryScrapeMsg struct {
	tags []string
	err  error
}

type registryProgressMsg struct {
	status string
	ch     chan string
}

type appModel struct {
	client          *OllamaClient
	list            list.Model
	appList         list.Model
	registryList    list.Model
	input           textinput.Model
	progress        progress.Model
	state           uiState
	width           int
	height          int
	logs            []logMsg
	pullStatus      string
	pullChan        chan pullResponse
	pullCompleted   int64
	pullTotal       int64
	pullCancel      context.CancelFunc
	chatCmdTemplate string
}

func newAppModel() *appModel {
	client, _ := NewOllamaClient()

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Models"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	ti := textinput.New()
	ti.Placeholder = "Enter model name (e.g. model or model --insecure)"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 30

	prog := progress.New(progress.WithGradient("#fabd2f", "#fe8019"))

	// Determine chat command from environment, default to ollama run
	chatCmdTemplate := os.Getenv("LAZYLLM_CHAT_CMD")
	if chatCmdTemplate == "" {
		chatCmdTemplate = "ollama run {model}"
	}

	// Initialize the Open With list
	appItems := []list.Item{
		appItem{name: "Claude Code", cmd: "ollama launch claude --model {model}"},
		appItem{name: "Codex", cmd: "ollama launch codex --model {model}"},
		appItem{name: "OpenCode", cmd: "ollama launch opencode --model {model}"},
		appItem{name: "OpenClaw", cmd: "ollama launch openclaw --model {model}"},
	}
	al := list.New(appItems, list.NewDefaultDelegate(), 0, 0)
	al.Title = "Open With..."
	al.SetShowStatusBar(false)
	al.SetFilteringEnabled(false)
	al.Styles.Title = titleStyle

	rl := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	rl.Title = "Ollama Registry"
	rl.SetShowStatusBar(true)
	rl.SetFilteringEnabled(true)
	rl.Styles.Title = titleStyle

	return &appModel{
		client:          client,
		list:            l,
		appList:         al,
		registryList:    rl,
		input:           ti,
		progress:        prog,
		state:           stateBrowsing,
		logs:            []logMsg{{text: "Welcome to lazyllm", level: "info"}},
		chatCmdTemplate: chatCmdTemplate,
	}
}

func (m *appModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadModels(),
		textinput.Blink,
	)
}

type ollamaStartedMsg struct{}
type ollamaInstalledMsg struct{}

func (m *appModel) loadModels() tea.Cmd {
	return func() tea.Msg {
		// First check if ollama is even in the PATH
		if _, err := exec.LookPath("ollama"); err != nil {
			return errMsg(fmt.Errorf("ollama not installed"))
		}

		models, err := m.client.List()
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") {
				return errMsg(fmt.Errorf("ollama not running"))
			}
			return errMsg(err)
		}
		return loadModelsMsg{models: models}
	}
}

func (m *appModel) deleteSelected() tea.Cmd {
	if i, ok := m.list.SelectedItem().(modelItem); ok {
		return func() tea.Msg {
			err := m.client.Delete(i.model.Name)
			if err != nil {
				return errMsg(err)
			}
			return logMsg{text: fmt.Sprintf("Deleted model %s", i.model.Name), level: "success"}
		}
	}
	return nil
}

func (m *appModel) loadSelected() tea.Cmd {
	if i, ok := m.list.SelectedItem().(modelItem); ok {
		return func() tea.Msg {
			err := m.client.Load(i.model.Name)
			if err != nil {
				return errMsg(err)
			}
			return logMsg{text: fmt.Sprintf("Loaded model %s into memory", i.model.Name), level: "success"}
		}
	}
	return nil
}

func (m *appModel) unloadSelected() tea.Cmd {
	if i, ok := m.list.SelectedItem().(modelItem); ok {
		return func() tea.Msg {
			err := m.client.Unload(i.model.Name)
			if err != nil {
				return errMsg(err)
			}
			return logMsg{text: fmt.Sprintf("Unloaded model %s from memory", i.model.Name), level: "success"}
		}
	}
	return nil
}

// Removed pullModel from appModel

func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		h, v := docStyle.GetFrameSize()
		m.list.SetSize(m.width/3-h-2, m.height-12-v)
		m.appList.SetSize(m.width-m.width/3-6, m.height-18-v)
		m.registryList.SetSize(m.width-m.width/3-6, m.height-18-v)
		m.progress.Width = m.width - 4

	case loadModelsMsg:
		items := make([]list.Item, len(msg.models))
		for i, mod := range msg.models {
			items[i] = modelItem{model: mod}
		}
		cmd = m.list.SetItems(items)
		cmds = append(cmds, cmd)

	case logMsg:
		m.logs = append(m.logs, msg)
		if len(m.logs) > 5 {
			m.logs = m.logs[len(m.logs)-5:]
		}
		cmds = append(cmds, m.loadModels())

	case errMsg:
		if msg.Error() == "ollama not installed" {
			m.state = stateOllamaNotInstalled
		} else if msg.Error() == "ollama not running" {
			m.state = stateOllamaNotRunning
		} else {
			m.logs = append(m.logs, logMsg{text: msg.Error(), level: "error"})
			if len(m.logs) > 5 {
				m.logs = m.logs[len(m.logs)-5:]
			}
		}

	case registryScrapeMsg:
		if msg.err != nil {
			m.logs = append(m.logs, logMsg{text: fmt.Sprintf("Scrape error: %v", msg.err), level: "error"})
			m.registryList.SetItems([]list.Item{})
			m.registryList.Title = "Registry (Failed)"
		} else {
			items := make([]list.Item, len(msg.tags))
			for i, t := range msg.tags {
				items[i] = registryItem{tag: t}
			}
			m.registryList.SetItems(items)
			m.registryList.Title = fmt.Sprintf("Ollama Registry (%d tags)", len(msg.tags))
			m.logs = append(m.logs, logMsg{text: "Successfully fetched registry tags", level: "success"})
		}

	case registryProgressMsg:
		m.registryList.Title = msg.status
		cmds = append(cmds, func() tea.Msg {
			status, ok := <-msg.ch
			if !ok {
				return nil
			}
			return registryProgressMsg{status: status, ch: msg.ch}
		})
		return m, tea.Batch(cmds...)

	case ollamaInstalledMsg:
		m.state = stateOllamaNotRunning
		m.logs = append(m.logs, logMsg{text: "Successfully installed Ollama", level: "success"})
		cmds = append(cmds, m.loadModels())

	case ollamaStartedMsg:
		m.state = stateBrowsing
		m.logs = append(m.logs, logMsg{text: "Started ollama daemon", level: "success"})
		cmds = append(cmds, m.loadModels())

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case pullProgressMsg:
		if msg.err != nil {
			m.state = stateBrowsing
			if msg.err != context.Canceled {
				m.logs = append(m.logs, logMsg{text: msg.err.Error(), level: "error"})
			} else {
				m.logs = append(m.logs, logMsg{text: "Pull cancelled", level: "info"})
			}
			m.input.SetValue("")
			m.pullChan = nil
			if m.pullCancel != nil {
				m.pullCancel()
				m.pullCancel = nil
			}
			cmds = append(cmds, m.loadModels())
			return m, tea.Batch(cmds...)
		}
		if msg.done {
			m.state = stateBrowsing
			m.logs = append(m.logs, logMsg{text: "Pull completed", level: "success"})
			m.input.SetValue("")
			m.pullChan = nil
			if m.pullCancel != nil {
				m.pullCancel()
				m.pullCancel = nil
			}
			cmds = append(cmds, m.loadModels())
			return m, tea.Batch(cmds...)
		}

		m.pullStatus = msg.progress.Status
		if msg.progress.Total > 0 {
			m.pullTotal = msg.progress.Total
			m.pullCompleted = msg.progress.Completed
			cmd = m.progress.SetPercent(float64(m.pullCompleted) / float64(m.pullTotal))
			cmds = append(cmds, cmd)
		}

		if m.pullChan != nil {
			cmds = append(cmds, waitForProgress(m.pullChan))
		}
		return m, tea.Batch(cmds...)

	case pullCompleteMsg:
		m.state = stateBrowsing
		m.logs = append(m.logs, logMsg{text: "Pull completed", level: "success"})
		m.input.SetValue("")
		cmds = append(cmds, m.loadModels())

	case tea.KeyMsg:
		if m.state == stateOllamaNotInstalled {
			switch msg.String() {
			case "y", "Y", "enter":
				m.logs = append(m.logs, logMsg{text: "Installing Ollama...", level: "info"})

				c := exec.Command("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
				cmd = tea.ExecProcess(c, func(err error) tea.Msg {
					if err != nil {
						return errMsg(err)
					}
					return ollamaInstalledMsg{}
				})
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			case "n", "N", "q", "esc", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == stateRegistry {
			if m.registryList.FilterState() == list.Filtering {
				if msg.String() == "ctrl+c" {
					return m, tea.Quit
				}
				m.registryList, cmd = m.registryList.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}

			switch msg.String() {
			case "esc":
				if m.registryList.FilterState() == list.FilterApplied {
					m.registryList.ResetFilter()
					return m, nil
				}
				m.state = stateBrowsing
				return m, nil
			case "enter":
				if i, ok := m.registryList.SelectedItem().(registryItem); ok {
					m.input.SetValue(i.tag)
					m.state = statePulling
					return m, func() tea.Msg { return tea.KeyMsg{Type: tea.KeyEnter} }
				}
			}
			m.registryList, cmd = m.registryList.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		if m.state == stateOpenWith {
			if m.appList.FilterState() == list.Filtering {
				if msg.String() == "ctrl+c" {
					return m, tea.Quit
				}
				m.appList, cmd = m.appList.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}

			switch msg.String() {
			case "esc":
				if m.appList.FilterState() == list.FilterApplied {
					m.appList.ResetFilter()
					return m, nil
				}
				m.state = stateBrowsing
				return m, nil
			case "enter":
				if i, ok := m.list.SelectedItem().(modelItem); ok {
					if app, appOk := m.appList.SelectedItem().(appItem); appOk {
						cmdStr := strings.ReplaceAll(app.cmd, "{model}", i.model.Name)
						args := strings.Fields(cmdStr)

						if len(args) == 0 {
							cmds = append(cmds, func() tea.Msg { return errMsg(fmt.Errorf("invalid app command template")) })
							m.state = stateBrowsing
							return m, tea.Batch(cmds...)
						}

						c := exec.Command(args[0], args[1:]...)
						cmd = tea.ExecProcess(c, func(err error) tea.Msg {
							if err != nil {
								return errMsg(err)
							}
							return logMsg{text: fmt.Sprintf("Finished running %s", app.name), level: "info"}
						})
						m.state = stateBrowsing
						cmds = append(cmds, cmd)
						return m, tea.Batch(cmds...)
					}
				}
			}
			m.appList, cmd = m.appList.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		if m.state == statePulling {
			switch msg.Type {
			case tea.KeyEnter:
				if m.pullChan != nil {
					return m, nil // Ignore enter if already pulling
				}

				inputVal := strings.TrimSpace(m.input.Value())
				insecure := false
				modelName := inputVal

				if strings.HasSuffix(inputVal, "--insecure") {
					insecure = true
					modelName = strings.TrimSpace(strings.TrimSuffix(inputVal, "--insecure"))
				}

				m.logs = append(m.logs, logMsg{text: fmt.Sprintf("Pulling %s (insecure: %v)...", modelName, insecure), level: "info"})

				m.pullChan = make(chan pullResponse, 100) // Buffer it to prevent strict blocking
				m.pullStatus = "Starting pull..."
				m.pullCompleted = 0
				m.pullTotal = 0

				ctx, cancel := context.WithCancel(context.Background())
				m.pullCancel = cancel

				pullCh := m.pullChan
				go func() {
					err := m.client.Pull(ctx, modelName, insecure, func(resp api.ProgressResponse) error {
						// Don't block if the UI is slow/cancelled
						select {
						case pullCh <- pullResponse{progress: resp}:
						case <-ctx.Done():
							return ctx.Err()
						default:
						}
						return nil
					})

					select {
					case <-ctx.Done():
					case pullCh <- pullResponse{err: err, done: err == nil}:
					default:
					}
				}()

				return m, waitForProgress(m.pullChan)

			case tea.KeyEsc, tea.KeyCtrlC:
				if m.pullCancel != nil {
					m.pullCancel()
					m.pullCancel = nil
				}
				if m.pullChan != nil {
					m.pullChan = nil
				}
				m.state = stateBrowsing
				m.input.SetValue("")
				return m, nil
			}

			if m.pullChan == nil {
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		if m.state == stateBrowsing {
			if m.list.FilterState() == list.Filtering {
				if msg.String() == "ctrl+c" {
					return m, tea.Quit
				}
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}

			switch msg.String() {
			case "esc":
				if m.list.FilterState() == list.FilterApplied {
					m.list.ResetFilter()
				}
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Quit
			case "d":
				cmds = append(cmds, m.deleteSelected())
			case "l":
				cmds = append(cmds, m.loadSelected())
			case "u":
				cmds = append(cmds, m.unloadSelected())
			case "o":
				if m.list.SelectedItem() != nil {
					m.state = stateOpenWith
				}
				return m, nil
			case "p":
				m.state = statePulling
				m.input.Focus()
				return m, nil
			case "r":
				m.state = stateRegistry
				if len(m.registryList.Items()) == 0 {
					m.registryList.Title = "Gathering available models..."
					progressChan := make(chan string)

					var waitForProgress func(ch chan string) tea.Cmd
					waitForProgress = func(ch chan string) tea.Cmd {
						return func() tea.Msg {
							status, ok := <-ch
							if !ok {
								return nil
							}
							return registryProgressMsg{status: status, ch: ch}
						}
					}
					cmds = append(cmds, waitForProgress(progressChan))

					cmds = append(cmds, func() tea.Msg {
						tags, err := ScrapeAll(progressChan)
						close(progressChan)
						return registryScrapeMsg{tags: tags, err: err}
					})
				}
				return m, tea.Batch(cmds...)
			case "enter":
				if i, ok := m.list.SelectedItem().(modelItem); ok {
					cmdStr := strings.ReplaceAll(m.chatCmdTemplate, "{model}", i.model.Name)
					args := strings.Fields(cmdStr)

					if len(args) == 0 {
						cmds = append(cmds, func() tea.Msg { return errMsg(fmt.Errorf("invalid chat command template")) })
						return m, tea.Batch(cmds...)
					}

					c := exec.Command(args[0], args[1:]...)
					cmd = tea.ExecProcess(c, func(err error) tea.Msg {
						if err != nil {
							return errMsg(err)
						}
						return logMsg{text: "Chat ended", level: "info"}
					})
					cmds = append(cmds, cmd)
				}
			}

			m.list, cmd = m.list.Update(msg)
			cmds = append(cmds, cmd)
		}

	default:
		// Route internal unhandled messages (like background filter results) to the active lists
		var cmdList, cmdApp, cmdReg tea.Cmd
		m.list, cmdList = m.list.Update(msg)
		m.appList, cmdApp = m.appList.Update(msg)
		m.registryList, cmdReg = m.registryList.Update(msg)
		cmds = append(cmds, cmdList, cmdApp, cmdReg)
	}

	return m, tea.Batch(cmds...)
}

func (m *appModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.state == stateOllamaNotRunning {
		dialogBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(1, 2).
			Render(fmt.Sprintf("Ollama is not running.\n\nWould you like lazyllm to start it for you?\n\n[y/enter] Yes    [n/q] No (Quit)"))

		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialogBox)
	}

	paneHeight := m.height - 12
	if paneHeight < 5 {
		paneHeight = 5 // Safe minimum
	}

	// Models Pane
	modelsView := activePaneStyle.Width(m.width/3 - 2).Height(paneHeight).Render(m.list.View())

	// Details Pane
	detailsView := ""
	if m.state == statePulling {
		if m.pullChan != nil { // actively pulling
			var progressStr string
			if m.pullTotal > 0 {
				progressStr = fmt.Sprintf("%.2f GB / %.2f GB", float64(m.pullCompleted)/(1024*1024*1024), float64(m.pullTotal)/(1024*1024*1024))
			}
			detailsView = fmt.Sprintf(
				"Pulling Model: %s\n\nStatus: %s\n%s\n\n%s\n\n(esc to return to browsing)",
				m.input.Value(),
				m.pullStatus,
				progressStr,
				m.progress.View(),
			)
		} else { // waiting for input
			detailsView = fmt.Sprintf(
				"Pull Model\n\n%s\n\n(esc to cancel, enter to pull)",
				m.input.View(),
			)
		}
	} else if m.state == stateOpenWith {
		detailsView = m.appList.View()
	} else if m.state == stateRegistry {
		detailsView = m.registryList.View()
	} else if i, ok := m.list.SelectedItem().(modelItem); ok {
		sizeStr := fmt.Sprintf("%.2f GB", float64(i.model.Size)/(1024*1024*1024))
		detailsView = fmt.Sprintf(
			"Name: %s\nSize: %s\nFamily: %s\nFormat: %s\nQuantization: %s\nModified: %s",
			i.model.Name, sizeStr, i.model.Details.Family, i.model.Details.Format, i.model.Details.QuantizationLevel, i.model.ModifiedAt.Format("2006-01-02 15:04:05"),
		)
	} else {
		detailsView = "No models found."
	}

	actionsView := "Actions:\n[p] Pull  [r] Registry  [d] Delete  [l] Load  [u] Unload  [enter] Chat  [o] Open  [q] Quit"

	rightPaneView := paneStyle.Width(m.width - m.width/3 - 4).Height(paneHeight).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Height(paneHeight-4).Render(detailsView),
			actionsView,
		),
	)

	topSection := lipgloss.JoinHorizontal(lipgloss.Top, modelsView, rightPaneView)

	// Logs Pane
	logsText := ""
	for _, l := range m.logs {
		style := infoLogStyle
		switch l.level {
		case "success":
			style = successLogStyle
		case "error":
			style = errorLogStyle
		}
		logsText += style.Render(fmt.Sprintf("[%s] %s\n", strings.ToUpper(l.level), l.text))
	}

	logsPane := paneStyle.Width(m.width - 2).Height(8).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			statusStyle.Render("Logs / Status"),
			logsText,
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left, topSection, logsPane)
}
