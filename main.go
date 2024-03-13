package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	instruments []instrument
	margin      = lipgloss.NewStyle().Margin(1, 2, 0, 2)
)

type instrument struct {
	InstrumentName      string `json:"instrument_name"`
	ExpirationTimestamp int64  `json:"expiration_timestamp"`
}

type instrumentsResponse struct {
	Result []instrument `json:"result"`
}

type ticker struct {
	MarkPrice  float64 `json:"mark_price"`
	IndexPrice float64 `json:"index_price"`
}

type tickerResponse struct {
	Result ticker `json:"result"`
}

type tickerMsg struct {
	i instrument
	t ticker
}

type model struct {
	tickers map[instrument]ticker
	m       sync.Mutex
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
	case tickerMsg:
		tm := tickerMsg(msg)
		m.m.Lock()
		m.tickers[tm.i] = tm.t
		m.m.Unlock()
	}

	return m, nil
}

func getJSON(path string, params url.Values, response interface{}) error {
	u := url.URL{
		Scheme:   "https",
		Host:     "www.deribit.com",
		Path:     path,
		RawQuery: params.Encode()}

	resp, err := http.Get(u.String())
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	json.Unmarshal(body, &response)
	return nil
}

func getInstruments() []instrument {
	var response instrumentsResponse
	err := getJSON(
		"/api/v2/public/get_instruments",
		url.Values{"currency": {"BTC"}, "kind": {"future"}},
		&response)
	if err != nil {
		return []instrument{}
	}

	results := response.Result
	sort.Slice(results, func(i, j int) bool {
		return results[i].ExpirationTimestamp < results[j].ExpirationTimestamp
	})

	perpetual := results[len(results)-1:] // perp has longest duration
	futures := results[:len(results)-1]

	return append(perpetual, futures...)
}

func tenor(ms int64) string {
	days := ms / (1000 * 60 * 60 * 24)
	hours := (ms / (1000 * 60 * 60)) % 24
	minutes := (ms / (1000 * 60)) % 60

	return fmt.Sprintf("%3dd %2dh %2dm", days, hours, minutes)
}

func getTicker(i instrument) ticker {
	var response tickerResponse
	err := getJSON(
		"/api/v2/public/ticker",
		url.Values{"instrument_name": {i.InstrumentName}},
		&response)
	if err != nil {
		return ticker{}
	}

	return response.Result
}

func (m model) View() string {
	var output string

	for _, i := range instruments {
		m.m.Lock()
		t := m.tickers[i]
		m.m.Unlock()

		msToExpiration := i.ExpirationTimestamp - time.Now().UnixMilli()

		var premium, yield, annualisedYield float64

		if (t != ticker{}) {
			premium = t.MarkPrice - t.IndexPrice
			yield = premium / t.IndexPrice
			annualisedYield = yield / (float64(msToExpiration) / (1000 * 60 * 60 * 24 * 365))
		}

		if i.InstrumentName == "BTC-PERPETUAL" {
			output += fmt.Sprintf("%-13s %8.2f\n", i.InstrumentName, premium)
		} else {
			output += fmt.Sprintf("%-13s %8.2f %5.2f%% %s\n", i.InstrumentName, premium, annualisedYield*100, tenor(msToExpiration))
		}
	}

	return margin.Render(output)
}

func main() {
	m := model{tickers: map[instrument]ticker{}}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithFPS(10))

	instruments = getInstruments()
	for _, i := range instruments {
		go func(in instrument) {
			t := time.NewTicker(1 * time.Second)

			for {
				p.Send(tickerMsg{i: in, t: getTicker(in)})
				<-t.C
			}
		}(i)
	}

	if err := p.Start(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
