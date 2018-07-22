package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	cc "./cryptocompare"
	"github.com/Knetic/govaluate"
	talib "github.com/markcheno/go-talib"
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
	Checks []check `json:"checks"`
	Scope  string  `json:"scope"`
	Length int     `json:"length"`
}

func last(ary []float64) float64 {
	return ary[len(ary)-1]
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

		data := cc.Histoday(c.Coin, c.Currency, config.Length, c.Exchange).Data
		close := cc.Close(data)

		result[c.Name]["close"] = close

		params := make(map[string]interface{}, 16)
		params["coin"] = c.Coin
		params["currency"] = c.Currency
		params["length"] = config.Length
		params["close"] = last(close)

		for j := range c.Indicators {
			idc := c.Indicators[j]

			switch idc.Type {
			case "rsi":
				rsi := talib.Rsi(close, idc.Params[0])
				result[c.Name][idc.Name] = rsi
				params[idc.Name] = last(rsi)
			case "ema":
				ema := talib.Ema(close, idc.Params[0])
				result[c.Name][idc.Name] = ema
				params[idc.Name] = last(ema)
			case "macd":
				_, _, hist := talib.Macd(close, idc.Params[0], idc.Params[1], idc.Params[2])
				result[c.Name][idc.Name] = hist
				params[idc.Name] = last(hist)
			default:
			}
		}

		for j := range c.Alerts {
			alert := c.Alerts[j]

			switch alert.Type {
			case "calc":
				exp, err := govaluate.NewEvaluableExpression(alert.Code)
				res, err := exp.Evaluate(params)

				if err != nil {
					log.Fatal(err)
				}

				if res == true {
					fmt.Println(alert.Name)
				}
			default:
			}
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.Encode(result)
}
