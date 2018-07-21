package cryptocompare

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

// Tick data structure
type Tick struct {
	Time       int     `json:"time"`
	Close      float64 `json:"close"`
	High       float64 `json:"high"`
	Low        float64 `json:"low"`
	Open       float64 `json:"open"`
	VolumeFrom float64 `json:"volumefrom"`
	VolumeTo   float64 `json:"volumeto"`
}

// Historical data structure
type Historical struct {
	Data              []Tick
	Response          string
	Type              int
	Aggregated        bool
	TimeTo            int
	TimeFrom          int
	FirstValueInArray bool
	ConversionType    struct {
		Type             string `json:"type"`
		ConversionSymbol string `json:"conversionSymbol"`
	}
}

var (
	baseURL = "https://min-api.cryptocompare.com/data"
)

func query(q string, params []string) []byte {
	url := fmt.Sprintf("%s/%s?%s", baseURL, q, strings.Join(params, "&"))

	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	return body
}

// Close gets the close price from Data returned by the API
func Close(data []Tick) []float64 {
	result := []float64{}
	for i := range data {
		result = append(result, data[i].Close)
	}

	return result
}

// Histoday https://min-api.cryptocompare.com
func Histoday(fsym string, tsym string, limit int, e string) *Historical {
	var params []string
	params = append(params, fmt.Sprintf("fsym=%s", fsym))
	params = append(params, fmt.Sprintf("tsym=%s", tsym))
	params = append(params, fmt.Sprintf("limit=%v", limit))
	params = append(params, fmt.Sprintf("e=%s", e))

	body := query("histoday", params)

	jsonData := &Historical{}

	err := json.Unmarshal([]byte(body), &jsonData)
	if err != nil {
		log.Fatal(err)
	}

	return jsonData
}

// Histohour https://min-api.cryptocompare.com
func Histohour(fsym string, tsym string, limit int, e string) *Historical {
	var params []string
	params = append(params, fmt.Sprintf("fsym=%s", fsym))
	params = append(params, fmt.Sprintf("tsym=%s", tsym))
	params = append(params, fmt.Sprintf("limit=%v", limit))
	params = append(params, fmt.Sprintf("e=%s", e))

	body := query("histohour", params)

	jsonData := &Historical{}

	err := json.Unmarshal([]byte(body), &jsonData)
	if err != nil {
		log.Fatal(err)
	}

	return jsonData
}
