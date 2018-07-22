package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

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
	Alerts  []alert `json:"alerts"`
	Scope   string  `json:"scope"`
	Length  int     `json:"length"`
	Verbose bool    `json:"verbose"`
}

type notification struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

type params map[string]interface{}

var (
	luaResult = false
)

func luaAlertCallback(value bool) {
	luaResult = value
}

func reverse(numbers []float64) {
	for i, j := 0, len(numbers)-1; i < j; i, j = i+1, j-1 {
		numbers[i], numbers[j] = numbers[j], numbers[i]
	}
}

func setGlobal(L *lua.LState, name string, value interface{}) {
	L.SetGlobal(name, luar.New(L, value))
}

func runAlert(L *lua.LState, P params, alert alert) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = alert.Name
	n.Source = alert.Code

	switch alert.Type {
	case "lua":
		err := L.DoString(alert.Code)

		if err != nil {
			log.Fatal(err)
		}

		if luaResult {
			fired = true
			luaResult = false
		}
	case "calc":
		exp, err := govaluate.NewEvaluableExpression(alert.Code)
		res, err := exp.Evaluate(P)

		if err != nil {
			log.Fatal(err)
		}

		if res == true {
			fired = true
		}
	default:
	}

	return fired, n
}

func setIndicatorResult(src []float64, cName string, idcName string, L *lua.LState, Lg *lua.LState, P params, Pg params) {
	Pg[cName+"_"+idcName] = src[0]
	P[idcName] = src[0]
	setGlobal(Lg, cName+"_"+idcName, src)
	setGlobal(L, idcName, src)
}

func loadConfig(file string) {
	configFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}

	jsonParser := json.NewDecoder(configFile)
	err = jsonParser.Decode(&config)
	if err != nil {
		log.Fatal(err)
	}
}

func mainLoop(result map[string]map[string][]float64, Lg *lua.LState, Pg params) []notification {
	var notifications []notification

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

		Pg[c.Name+"_coin"] = c.Coin
		Pg[c.Name+"_currency"] = c.Currency

		Pg[c.Name+"_open"] = open[0]
		Pg[c.Name+"_high"] = high[0]
		Pg[c.Name+"_low"] = low[0]
		Pg[c.Name+"_close"] = close[0]

		P := make(params, 64)

		P["coin"] = c.Coin
		P["currency"] = c.Currency
		P["length"] = config.Length

		P["open"] = open[0]
		P["high"] = high[0]
		P["low"] = low[0]
		P["close"] = close[0]

		setGlobal(Lg, c.Name+"_coin", c.Coin)
		setGlobal(Lg, c.Name+"_currency", c.Currency)

		setGlobal(Lg, c.Name+"_open", open)
		setGlobal(Lg, c.Name+"_high", high)
		setGlobal(Lg, c.Name+"_low", low)
		setGlobal(Lg, c.Name+"_close", close)

		L := lua.NewState()
		defer L.Close()

		setGlobal(L, "alert", luaAlertCallback)

		setGlobal(L, "coin", c.Coin)
		setGlobal(L, "currency", c.Currency)
		setGlobal(L, "length", config.Length)

		setGlobal(L, "open", open)
		setGlobal(L, "high", high)
		setGlobal(L, "low", low)
		setGlobal(L, "close", close)

		for j := range c.Indicators {
			idc := c.Indicators[j]

			switch idc.Type {
			case "rsi":
				rsi := talib.Rsi(src, idc.Params[0])
				reverse(rsi)

				setIndicatorResult(rsi, c.Name, idc.Name, L, Lg, P, Pg)

				result[c.Name][idc.Name] = rsi
			case "ema":
				ema := talib.Ema(src, idc.Params[0])
				reverse(ema)

				setIndicatorResult(ema, c.Name, idc.Name, L, Lg, P, Pg)

				result[c.Name][idc.Name] = ema
			case "macd":
				_, _, hist := talib.Macd(src, idc.Params[0], idc.Params[1], idc.Params[2])
				reverse(hist)

				setIndicatorResult(hist, c.Name, idc.Name, L, Lg, P, Pg)

				result[c.Name][idc.Name] = hist
			default:
			}
		}

		for j := range c.Alerts {
			fired, n := runAlert(L, P, c.Alerts[j])

			if fired {
				notifications = append(notifications, n)
			}
		}
	}

	for j := range config.Alerts {
		fired, n := runAlert(Lg, Pg, config.Alerts[j])

		if fired {
			notifications = append(notifications, n)
		}
	}

	return notifications
}

func main() {
	if len(os.Args) >= 2 {
		loadConfig(os.Args[1])
	} else {
		loadConfig("config.json")
	}

	result := make(map[string]map[string][]float64)

	Pg := make(params, 64)
	Pg["length"] = config.Length

	Lg := lua.NewState()
	defer Lg.Close()

	setGlobal(Lg, "alert", luaAlertCallback)
	setGlobal(Lg, "length", config.Length)

	notifications := mainLoop(result, Lg, Pg)

	if config.Verbose {
		output := make(map[string]interface{})

		output["data"] = result
		output["alerts"] = notifications

		enc := json.NewEncoder(os.Stdout)
		enc.Encode(output)
	}
}
