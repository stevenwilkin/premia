package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

var (
	instruments []instrument
)

type instrument struct {
	InstrumentName      string `json:"instrument_name"`
	ExpirationTimestamp int64  `json:"expiration_timestamp"`
}

type instrumentsResponse struct {
	Result []instrument `json:"result"`
}

type tickerResponse struct {
	Result struct {
		MarkPrice  float64 `json:"mark_price"`
		IndexPrice float64 `json:"index_price"`
	} `json:"result"`
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

func getYields() {
	for _, i := range instruments {
		var response tickerResponse
		err := getJSON(
			"/api/v2/public/ticker",
			url.Values{"instrument_name": {i.InstrumentName}},
			&response)
		if err != nil {
			return
		}

		msToExpiration := i.ExpirationTimestamp - time.Now().UnixMilli()

		premium := response.Result.MarkPrice - response.Result.IndexPrice
		yield := premium / response.Result.IndexPrice
		annualisedYield := yield / (float64(msToExpiration) / (1000 * 60 * 60 * 24 * 365))

		if i.InstrumentName == "BTC-PERPETUAL" {
			fmt.Printf("%-13s %7.2f\n", i.InstrumentName, premium)
		} else {
			fmt.Printf("%-13s %7.2f %5.2f%% %s\n", i.InstrumentName, premium, annualisedYield*100, tenor(msToExpiration))
		}
	}
}

func main() {
	instruments = getInstruments()
	getYields()
}
