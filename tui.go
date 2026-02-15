package main

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const scanTimeout = 5 * time.Second

type state int

const (
	stateScanning  state = iota
	stateSelecting
	stateDone
)

type scanDoneMsg struct {
	bridges []Bridge
	err     error
}

type model struct {
	state    state
	spinner  spinner.Model
	bridges  []Bridge
	cursor   int
	selected *Bridge
	err      error
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	itemStyle     = lipgloss.NewStyle().PaddingLeft(2)
	selectedStyle = lipgloss.NewStyle().PaddingLeft(0).Foreground(lipgloss.Color("170"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func newModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	return model{
		state:   stateScanning,
		spinner: s,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, scanCmd())
}

func scanCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()

		bridgeCh, errCh := DiscoverBridges(ctx)

		var bridges []Bridge
		for b := range bridgeCh {
			bridges = append(bridges, b)
		}

		if err := <-errCh; err != nil {
			return scanDoneMsg{err: err}
		}

		return scanDoneMsg{bridges: bridges}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scanDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}

		if len(msg.bridges) == 0 {
			m.err = fmt.Errorf("no Hue bridges found on the network")
			m.state = stateDone
			return m, tea.Quit
		}

		if len(msg.bridges) == 1 {
			m.selected = &msg.bridges[0]
			m.state = stateDone
			return m, tea.Quit
		}

		m.bridges = msg.bridges
		m.state = stateSelecting
		return m, nil
	}

	if m.state == stateSelecting {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.bridges)-1 {
					m.cursor++
				}
			case "enter":
				m.selected = &m.bridges[m.cursor]
				m.state = stateDone
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	switch m.state {
	case stateScanning:
		return fmt.Sprintf("\n %s %s\n\n",
			m.spinner.View(),
			titleStyle.Render("Scanning for Hue bridges..."))

	case stateSelecting:
		s := "\n" + titleStyle.Render("  Select a Hue Bridge:") + "\n\n"
		for i, b := range m.bridges {
			label := fmt.Sprintf("%s (%s) — %s", b.Name, b.ID, b.IP)
			if i == m.cursor {
				s += selectedStyle.Render("▸ "+label) + "\n"
			} else {
				s += itemStyle.Render(label) + "\n"
			}
		}
		s += "\n" + helpStyle.Render("  ↑/k up · ↓/j down · enter select · q quit") + "\n"
		return s

	case stateDone:
		if m.err != nil {
			return "\n" + errStyle.Render("  Error: "+m.err.Error()) + "\n\n"
		}
		if m.selected != nil {
			return fmt.Sprintf("\n  Selected bridge: %s\n\n", m.selected)
		}
	}

	return ""
}
