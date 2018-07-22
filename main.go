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
	expr map[string]interface{}
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

func luaState(L *lua.LState, name string, value interface{}) {
	L.SetGlobal(name, luar.New(L, value))
}

func exprState(P params, name string, value interface{}) {
	P[name] = value
}

func runAlert(state scriptState, alert alert) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = alert.Name
	n.Source = alert.Code

	luaResult := false

	switch alert.Type {
	case "lua":
		luaState(state.lua, "alert", func(v bool) { luaResult = v })

		err := state.lua.DoString(alert.Code)

		if err != nil {
			log.Fatal(err)
		}

		if luaResult {
			fired = true
			luaResult = false
		}
	case "expr":
		exp, err := govaluate.NewEvaluableExpression(alert.Code)
		res, err := exp.Evaluate(state.expr)

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

func setIndicatorResult(src []float64, cName string, idcName string, localState scriptState, globalState scriptState) {
	exprState(globalState.expr, cName+"_"+idcName, src[0])
	exprState(localState.expr, idcName, src[0])

	luaState(globalState.lua, cName+"_"+idcName, src)
	luaState(localState.lua, idcName, src)
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
	globalState.expr = make(params, 64)

	globalState.lua = lua.NewState()
	defer globalState.lua.Close()

	gExpr := globalState.expr
	gLua := globalState.lua

	exprState(gExpr, "length", config.Length)
	luaState(gLua, "length", config.Length)

	result := make(results)
	var notifications []notification

	for i := range config.Checks {
		c := config.Checks[i]

		result[c.Name] = make(map[string][]float64)
		var data []cc.Tick

		var localState scriptState

		localState.expr = make(params, 64)

		localState.lua = lua.NewState()
		defer localState.lua.Close()

		lExpr := localState.expr
		lLua := localState.lua

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

		exprState(gExpr, c.Name+"_coin", c.Coin)
		exprState(gExpr, c.Name+"_currency", c.Currency)

		exprState(gExpr, c.Name+"_open", open[0])
		exprState(gExpr, c.Name+"_high", high[0])
		exprState(gExpr, c.Name+"_low", low[0])
		exprState(gExpr, c.Name+"_close", close[0])

		exprState(lExpr, "coin", c.Coin)
		exprState(lExpr, "currency", c.Currency)
		exprState(lExpr, "length", config.Length)

		exprState(lExpr, "open", open[0])
		exprState(lExpr, "high", high[0])
		exprState(lExpr, "low", low[0])
		exprState(lExpr, "close", close[0])

		luaState(gLua, c.Name+"_coin", c.Coin)
		luaState(gLua, c.Name+"_currency", c.Currency)

		luaState(gLua, c.Name+"_open", open)
		luaState(gLua, c.Name+"_high", high)
		luaState(gLua, c.Name+"_low", low)
		luaState(gLua, c.Name+"_close", close)

		luaState(lLua, "coin", c.Coin)
		luaState(lLua, "currency", c.Currency)
		luaState(lLua, "length", config.Length)

		luaState(lLua, "open", open)
		luaState(lLua, "high", high)
		luaState(lLua, "low", low)
		luaState(lLua, "close", close)

		for j := range c.Indicators {
			idc := c.Indicators[j]

			switch idc.Type {
			case "rsi":
				rsi := talib.Rsi(src, idc.Params[0])
				reverse(rsi)

				setIndicatorResult(rsi, c.Name, idc.Name, localState, globalState)

				result[c.Name][idc.Name] = rsi
			case "ema":
				ema := talib.Ema(src, idc.Params[0])
				reverse(ema)

				setIndicatorResult(ema, c.Name, idc.Name, localState, globalState)

				result[c.Name][idc.Name] = ema
			case "macd":
				_, _, hist := talib.Macd(src, idc.Params[0], idc.Params[1], idc.Params[2])
				reverse(hist)

				setIndicatorResult(hist, c.Name, idc.Name, localState, globalState)

				result[c.Name][idc.Name] = hist
			default:
			}
		}

		for j := range c.Alerts {
			fired, n := runAlert(localState, c.Alerts[j])

			if fired {
				notifications = append(notifications, n)
			}
		}
	}

	for j := range config.Alerts {
		fired, n := runAlert(globalState, config.Alerts[j])

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
