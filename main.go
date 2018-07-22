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

type notification struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

type params map[string]interface{}

type results map[string]map[string][]float64

type scriptState struct {
	calc map[string]interface{}
	lua  *lua.LState
}

var config struct {
	Checks  []check `json:"checks"`
	Alerts  []alert `json:"alerts"`
	Scope   string  `json:"scope"`
	Length  int     `json:"length"`
	Verbose bool    `json:"verbose"`
}

func reverse(numbers []float64) {
	for i, j := 0, len(numbers)-1; i < j; i, j = i+1, j-1 {
		numbers[i], numbers[j] = numbers[j], numbers[i]
	}
}

func setLua(L *lua.LState, name string, value interface{}) {
	L.SetGlobal(name, luar.New(L, value))
}

func setCalc(P params, name string, value interface{}) {
	P[name] = value
}

func runAlert(state scriptState, L *lua.LState, P params, alert alert) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = alert.Name
	n.Source = alert.Code

	luaResult := false

	switch alert.Type {
	case "lua":
		setLua(L, "alert", func(v bool) { luaResult = v })

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
	setLua(Lg, cName+"_"+idcName, src)
	setLua(L, idcName, src)
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

func mainLoop(globalState scriptState) (results, []notification) {
	globalState.calc = make(params, 64)

	globalState.lua = lua.NewState()
	defer globalState.lua.Close()

	Pg := globalState.calc
	Lg := globalState.lua

	setCalc(Pg, "length", config.Length)
	setLua(Lg, "length", config.Length)

	result := make(results)
	var notifications []notification

	for i := range config.Checks {
		c := config.Checks[i]

		result[c.Name] = make(map[string][]float64)
		var data []cc.Tick

		var localState scriptState

		localState.calc = make(params, 64)

		localState.lua = lua.NewState()
		defer localState.lua.Close()

		P := localState.calc
		L := localState.lua

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

		setCalc(Pg, c.Name+"_coin", c.Coin)
		setCalc(Pg, c.Name+"_currency", c.Currency)

		setCalc(Pg, c.Name+"_open", open[0])
		setCalc(Pg, c.Name+"_high", high[0])
		setCalc(Pg, c.Name+"_low", low[0])
		setCalc(Pg, c.Name+"_close", close[0])

		setCalc(P, "coin", c.Coin)
		setCalc(P, "currency", c.Currency)
		setCalc(P, "length", config.Length)

		setCalc(P, "open", open[0])
		setCalc(P, "high", high[0])
		setCalc(P, "low", low[0])
		setCalc(P, "close", close[0])

		setLua(Lg, c.Name+"_coin", c.Coin)
		setLua(Lg, c.Name+"_currency", c.Currency)

		setLua(Lg, c.Name+"_open", open)
		setLua(Lg, c.Name+"_high", high)
		setLua(Lg, c.Name+"_low", low)
		setLua(Lg, c.Name+"_close", close)

		setLua(L, "coin", c.Coin)
		setLua(L, "currency", c.Currency)
		setLua(L, "length", config.Length)

		setLua(L, "open", open)
		setLua(L, "high", high)
		setLua(L, "low", low)
		setLua(L, "close", close)

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
			fired, n := runAlert(localState, L, P, c.Alerts[j])

			if fired {
				notifications = append(notifications, n)
			}
		}
	}

	for j := range config.Alerts {
		fired, n := runAlert(globalState, Lg, Pg, config.Alerts[j])

		if fired {
			notifications = append(notifications, n)
		}
	}

	return result, notifications
}

func main() {
	if len(os.Args) >= 2 {
		loadConfig(os.Args[1])
	} else {
		loadConfig("config.json")
	}

	var state scriptState

	result, notifications := mainLoop(state)

	output := make(map[string]interface{})
	output["alerts"] = notifications

	if config.Verbose {
		output["data"] = result
	}

	enc := json.NewEncoder(os.Stdout)
	enc.Encode(output)
}
