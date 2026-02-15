package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const scanTimeout = 5 * time.Second

type state int

const (
	stateScanning      state = iota
	stateSelecting
	statePairing
	statePairingWait
	stateFetchingAreas
	stateSelectingArea
	stateDone
)

type scanDoneMsg struct {
	bridges []Bridge
	err     error
}

type pairResultMsg struct {
	username  string
	clientkey string
	err       error
}

type areasFetchedMsg struct {
	areas []EntertainmentArea
	err   error
}

type model struct {
	state    state
	spinner  spinner.Model
	bridges  []Bridge
	cursor   int
	selected *Bridge
	err      error

	username     string
	clientkey    string
	pairErr      string
	areas        []EntertainmentArea
	areaCursor   int
	selectedArea *EntertainmentArea
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

func pairCmd(ip net.IP) tea.Cmd {
	return func() tea.Msg {
		username, clientkey, err := PairBridge(ip)
		return pairResultMsg{username: username, clientkey: clientkey, err: err}
	}
}

func fetchAreasCmd(ip net.IP, username string) tea.Cmd {
	return func() tea.Msg {
		areas, err := FetchEntertainmentAreas(ip, username)
		return areasFetchedMsg{areas: areas, err: err}
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
			if creds, found, _ := LoadCredentials(m.selected.ID); found {
				m.username = creds.Username
				m.clientkey = creds.Clientkey
				m.state = stateFetchingAreas
				return m, fetchAreasCmd(m.selected.IP, m.username)
			}
			m.state = statePairing
			return m, nil
		}

		m.bridges = msg.bridges
		m.state = stateSelecting
		return m, nil

	case pairResultMsg:
		if msg.err != nil {
			if errors.Is(msg.err, ErrLinkButtonNotPressed) {
				m.pairErr = "Link button not pressed."
				m.state = statePairing
				return m, nil
			}
			m.err = fmt.Errorf("pairing failed: %w", msg.err)
			m.state = stateDone
			return m, tea.Quit
		}
		m.username = msg.username
		m.clientkey = msg.clientkey
		m.pairErr = ""
		_ = SaveCredentials(m.selected.ID, BridgeCredentials{
			Username:  msg.username,
			Clientkey: msg.clientkey,
		})
		m.state = stateFetchingAreas
		return m, fetchAreasCmd(m.selected.IP, m.username)

	case areasFetchedMsg:
		if msg.err != nil {
			if errors.Is(msg.err, ErrUnauthorized) {
				_ = DeleteCredentials(m.selected.ID)
				m.username = ""
				m.clientkey = ""
				m.pairErr = "Stored credentials were rejected by the bridge."
				m.state = statePairing
				return m, nil
			}
			m.err = fmt.Errorf("fetching entertainment areas: %w", msg.err)
			m.state = stateDone
			return m, tea.Quit
		}

		if len(msg.areas) == 0 {
			m.err = fmt.Errorf("no entertainment areas configured on this bridge")
			m.state = stateDone
			return m, tea.Quit
		}

		if len(msg.areas) == 1 {
			m.selectedArea = &msg.areas[0]
			m.state = stateDone
			return m, tea.Quit
		}

		m.areas = msg.areas
		m.state = stateSelectingArea
		return m, nil
	}

	switch m.state {
	case stateSelecting:
		if msg, ok := msg.(tea.KeyMsg); ok {
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
				if creds, found, _ := LoadCredentials(m.selected.ID); found {
					m.username = creds.Username
					m.clientkey = creds.Clientkey
					m.state = stateFetchingAreas
					return m, fetchAreasCmd(m.selected.IP, m.username)
				}
				m.state = statePairing
			}
		}

	case statePairing:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "enter":
				m.state = statePairingWait
				return m, pairCmd(m.selected.IP)
			}
		}

	case stateSelectingArea:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "up", "k":
				if m.areaCursor > 0 {
					m.areaCursor--
				}
			case "down", "j":
				if m.areaCursor < len(m.areas)-1 {
					m.areaCursor++
				}
			case "enter":
				m.selectedArea = &m.areas[m.areaCursor]
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

	case statePairing:
		s := "\n"
		if m.pairErr != "" {
			s += errStyle.Render("  "+m.pairErr) + "\n\n"
		}
		s += titleStyle.Render("  Press the link button on your Hue bridge, then press Enter.") + "\n\n"
		s += helpStyle.Render("  enter pair · q quit") + "\n"
		return s

	case statePairingWait:
		return fmt.Sprintf("\n %s %s\n\n",
			m.spinner.View(),
			titleStyle.Render("Pairing with bridge..."))

	case stateFetchingAreas:
		return fmt.Sprintf("\n %s %s\n\n",
			m.spinner.View(),
			titleStyle.Render("Fetching entertainment areas..."))

	case stateSelectingArea:
		s := "\n" + titleStyle.Render("  Select an Entertainment Area:") + "\n\n"
		for i, a := range m.areas {
			label := a.String()
			if i == m.areaCursor {
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
		var s string
		if m.selected != nil {
			s += fmt.Sprintf("\n  Bridge: %s\n", m.selected)
		}
		if m.selectedArea != nil {
			s += fmt.Sprintf("  Area:   %s\n", m.selectedArea)
		}
		if s != "" {
			return s + "\n"
		}
	}

	return ""
}
