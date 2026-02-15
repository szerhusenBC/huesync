package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
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
	stateInputDelay
	stateActivating
	stateConnecting
	stateStreaming
	stateStopping
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

type activateResultMsg struct {
	err error
}

type connectResultMsg struct {
	streamer *Streamer
	err      error
}

type streamTickMsg struct{}

type frameSentMsg struct {
	color RGB
	err   error
}

type stopDoneMsg struct {
	err error
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

	delayInput   string
	captureDelay time.Duration

	streamer  *Streamer
	lastColor RGB
	streamErr error
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

func activateCmd(ip net.IP, username, areaID string) tea.Cmd {
	return func() tea.Msg {
		err := ActivateArea(ip, username, areaID)
		return activateResultMsg{err: err}
	}
}

func connectCmd(ip net.IP, username, clientkey, areaID string, channelIDs []uint8) tea.Cmd {
	return func() tea.Msg {
		streamer, err := NewStreamer(ip, username, clientkey, areaID, channelIDs)
		return connectResultMsg{streamer: streamer, err: err}
	}
}

func captureAndSendCmd(s *Streamer) tea.Cmd {
	return func() tea.Msg {
		img, err := CaptureScreen()
		if err != nil {
			return frameSentMsg{err: err}
		}
		color := AverageColor(img)
		err = s.SendColor(color)
		return frameSentMsg{color: color, err: err}
	}
}

func streamTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return streamTickMsg{}
	})
}

func stopCmd(s *Streamer, ip net.IP, username, areaID string) tea.Cmd {
	return func() tea.Msg {
		var firstErr error
		if s != nil {
			if err := s.Close(); err != nil {
				firstErr = err
			}
		}
		if err := DeactivateArea(ip, username, areaID); err != nil && firstErr == nil {
			firstErr = err
		}
		return stopDoneMsg{err: firstErr}
	}
}

func (m model) startStreaming() (model, tea.Cmd) {
	m.state = stateActivating
	return m, activateCmd(m.selected.IP, m.username, m.selectedArea.ID)
}

func (m model) enterDelayInput() (model, tea.Cmd) {
	m.delayInput = "100"
	m.state = stateInputDelay
	return m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == stateStreaming {
				m.state = stateStopping
				return m, stopCmd(m.streamer, m.selected.IP, m.username, m.selectedArea.ID)
			}
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
			return m.enterDelayInput()
		}

		m.areas = msg.areas
		m.state = stateSelectingArea
		return m, nil

	case activateResultMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("activating area: %w", msg.err)
			m.state = stateDone
			return m, tea.Quit
		}
		m.state = stateConnecting
		return m, connectCmd(m.selected.IP, m.username, m.clientkey, m.selectedArea.ID, m.selectedArea.ChannelIDs)

	case connectResultMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("connecting: %w", msg.err)
			m.state = stateStopping
			return m, stopCmd(nil, m.selected.IP, m.username, m.selectedArea.ID)
		}
		m.streamer = msg.streamer
		m.state = stateStreaming
		return m, captureAndSendCmd(m.streamer)

	case frameSentMsg:
		if msg.err != nil {
			m.streamErr = msg.err
		} else {
			m.lastColor = msg.color
			m.streamErr = nil
		}
		return m, streamTickCmd(m.captureDelay)

	case streamTickMsg:
		if m.state == stateStreaming {
			return m, captureAndSendCmd(m.streamer)
		}
		return m, nil

	case stopDoneMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.state = stateDone
		return m, tea.Quit
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
				return m.enterDelayInput()
			}
		}

	case stateInputDelay:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
				m.delayInput += msg.String()
			case "backspace":
				if len(m.delayInput) > 0 {
					m.delayInput = m.delayInput[:len(m.delayInput)-1]
				}
			case "enter":
				ms, err := strconv.Atoi(m.delayInput)
				if err != nil || ms <= 0 {
					ms = 100
				}
				m.captureDelay = time.Duration(ms) * time.Millisecond
				return m.startStreaming()
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

	case stateInputDelay:
		s := "\n" + titleStyle.Render("  Capture delay (ms):") + "\n\n"
		s += fmt.Sprintf("  > %s\n", m.delayInput)
		s += "\n" + helpStyle.Render("  type a number · enter confirm · q quit") + "\n"
		return s

	case stateActivating:
		return fmt.Sprintf("\n %s %s\n\n",
			m.spinner.View(),
			titleStyle.Render("Activating entertainment area..."))

	case stateConnecting:
		return fmt.Sprintf("\n %s %s\n\n",
			m.spinner.View(),
			titleStyle.Render("Connecting to bridge (DTLS)..."))

	case stateStreaming:
		s := "\n" + titleStyle.Render("  Streaming") + "\n\n"
		s += fmt.Sprintf("  Bridge: %s\n", m.selected)
		s += fmt.Sprintf("  Area:   %s\n", m.selectedArea)
		s += fmt.Sprintf("  Delay:  %dms\n", m.captureDelay.Milliseconds())
		s += fmt.Sprintf("  Color:  %s\n", m.lastColor)
		if m.streamErr != nil {
			s += errStyle.Render(fmt.Sprintf("  Error:  %s", m.streamErr)) + "\n"
		}
		s += "\n" + helpStyle.Render("  q quit") + "\n"
		return s

	case stateStopping:
		return fmt.Sprintf("\n %s %s\n\n",
			m.spinner.View(),
			titleStyle.Render("Stopping..."))

	case stateDone:
		if m.err != nil {
			return "\n" + errStyle.Render("  Error: "+m.err.Error()) + "\n\n"
		}
	}

	return ""
}
