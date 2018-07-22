package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	cc "./cryptocompare"
	"github.com/Knetic/govaluate"
	talib "github.com/markcheno/go-talib"
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

type indicator struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Params []int  `json:"params"`
}

type alert struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Code string `json:"code"`
}

type check struct {
	Name       string      `json:"name"`
	Coin       string      `json:"coin"`
	Currency   string      `json:"currency"`
	Exchange   string      `json:"exchange"`
	Indicators []indicator `json:"indicators"`
	Alerts     []alert     `json:"alerts"`
}

var config struct {
	Checks  []check `json:"checks"`
	Scope   string  `json:"scope"`
	Length  int     `json:"length"`
	Verbose bool    `json:"verbose"`
}

var (
	luaResult = false
)

func luaAlert(value bool) {
	luaResult = value
}

func reverse(numbers []float64) {
	for i, j := 0, len(numbers)-1; i < j; i, j = i+1, j-1 {
		numbers[i], numbers[j] = numbers[j], numbers[i]
	}
}

func main() {
	result := make(map[string]map[string][]float64)

	file := os.Args[1]

	configFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}

	jsonParser := json.NewDecoder(configFile)
	err = jsonParser.Decode(&config)
	if err != nil {
		log.Fatal(err)
	}

	for i := range config.Checks {
		c := config.Checks[i]
		result[c.Name] = make(map[string][]float64)

		var data []cc.Tick

		if config.Scope == "hour" {
			data = cc.Histohour(c.Coin, c.Currency, config.Length, c.Exchange).Data
		} else {
			data = cc.Histoday(c.Coin, c.Currency, config.Length, c.Exchange).Data
		}

		src := cc.Close(data)

		open := cc.Open(data)
		reverse(open)
		high := cc.High(data)
		reverse(high)
		low := cc.Low(data)
		reverse(low)
		close := cc.Close(data)
		reverse(close)

		result[c.Name]["open"] = open
		result[c.Name]["high"] = high
		result[c.Name]["low"] = low
		result[c.Name]["close"] = close

		params := make(map[string]interface{}, 64)
		params["coin"] = c.Coin
		params["currency"] = c.Currency
		params["length"] = config.Length
		params["open"] = open[0]
		params["high"] = high[0]
		params["low"] = low[0]
		params["close"] = close[0]

		L := lua.NewState()
		defer L.Close()

		L.SetGlobal("alert", luar.New(L, luaAlert))
		L.SetGlobal("open", luar.New(L, open))
		L.SetGlobal("high", luar.New(L, high))
		L.SetGlobal("low", luar.New(L, low))
		L.SetGlobal("close", luar.New(L, close))

		for j := range c.Indicators {
			idc := c.Indicators[j]

			switch idc.Type {
			case "rsi":
				rsi := talib.Rsi(src, idc.Params[0])
				reverse(rsi)

				result[c.Name][idc.Name] = rsi

				params[idc.Name] = rsi[0]
				L.SetGlobal(idc.Name, luar.New(L, rsi))
			case "ema":
				ema := talib.Ema(src, idc.Params[0])
				reverse(ema)

				result[c.Name][idc.Name] = ema

				params[idc.Name] = ema[0]
				L.SetGlobal(idc.Name, luar.New(L, ema))
			case "macd":
				_, _, hist := talib.Macd(src, idc.Params[0], idc.Params[1], idc.Params[2])
				reverse(hist)

				result[c.Name][idc.Name] = hist

				params[idc.Name] = hist[0]
				L.SetGlobal(idc.Name, luar.New(L, hist))
			default:
			}
		}

		for j := range c.Alerts {
			alert := c.Alerts[j]

			switch alert.Type {
			case "lua":
				err = L.DoString(alert.Code)

				if err != nil {
					log.Fatal(err)
				}

				if luaResult {
					fmt.Printf("%s: %s (Lua)\n", c.Name, alert.Name)
					luaResult = false
				}
			case "calc":
				exp, err := govaluate.NewEvaluableExpression(alert.Code)
				res, err := exp.Evaluate(params)

				if err != nil {
					log.Fatal(err)
				}

				if res == true {
					fmt.Printf("%s: %s (Calc)\n", c.Name, alert.Name)
				}
			default:
			}
		}

		if config.Verbose {
			enc := json.NewEncoder(os.Stdout)
			enc.Encode(result)
		}
	}
}
