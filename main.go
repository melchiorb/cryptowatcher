package main

import (
	"encoding/json"
	"fmt"
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
	Code      string `json:"code"`
}

type dataset map[string][]float64

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

type exprState map[string]interface{}

type scriptState struct {
	expr exprState
	lua  *lua.LState
}

func (state *scriptState) init() {
	state.expr = make(exprState, 64)
	state.lua = lua.NewState()
}

func (state *scriptState) close() {
	state.lua.Close()
}

func (state *scriptState) setLua(name string, value interface{}) {
	state.lua.SetGlobal(name, luar.New(state.lua, value))
}

func (state *scriptState) setExpr(name string, value interface{}) {
	state.expr[name] = value
}

func (state *scriptState) setAll(name string, value interface{}) {
	state.setExpr(name, value)
	state.setLua(name, value)
}

func runAlert(state scriptState, alert alert) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = alert.Name
	n.Code = alert.Code

	luaResult := false

	switch alert.Type {
	case "lua":
		state.setLua("alert", func(v bool) { luaResult = v })

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
	globalState.setExpr(cName+"_"+idcName, src[0])
	localState.setExpr(idcName, src[0])

	globalState.setLua(cName+"_"+idcName, src)
	localState.setLua(idcName, src)
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

func mainLoop(globalState scriptState, notifications chan<- notification, results chan<- dataset) {
	globalState.init()
	defer globalState.close()

	for i := range config.Checks {
		c := config.Checks[i]

		result := make(dataset)

		var data []cc.Tick

		var localState scriptState

		localState.init()
		defer localState.close()

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

		result["open"] = open
		result["high"] = high
		result["low"] = low
		result["close"] = close

		globalState.setAll(c.Name+"_coin", c.Coin)
		globalState.setAll(c.Name+"_currency", c.Currency)
		globalState.setAll("length", config.Length)

		localState.setAll("coin", c.Coin)
		localState.setAll("currency", c.Currency)
		localState.setAll("length", config.Length)

		globalState.setExpr(c.Name+"_open", open[0])
		globalState.setExpr(c.Name+"_high", high[0])
		globalState.setExpr(c.Name+"_low", low[0])
		globalState.setExpr(c.Name+"_close", close[0])

		localState.setExpr("open", open[0])
		localState.setExpr("high", high[0])
		localState.setExpr("low", low[0])
		localState.setExpr("close", close[0])

		globalState.setLua(c.Name+"_open", open)
		globalState.setLua(c.Name+"_high", high)
		globalState.setLua(c.Name+"_low", low)
		globalState.setLua(c.Name+"_close", close)

		localState.setLua("open", open)
		localState.setLua("high", high)
		localState.setLua("low", low)
		localState.setLua("close", close)

		for j := range c.Indicators {
			idc := c.Indicators[j]

			switch idc.Type {
			case "rsi":
				rsi := talib.Rsi(src, idc.Params[0])
				reverse(rsi)

				setIndicatorResult(rsi, c.Name, idc.Name, localState, globalState)

				result[idc.Name] = rsi
			case "ema":
				ema := talib.Ema(src, idc.Params[0])
				reverse(ema)

				setIndicatorResult(ema, c.Name, idc.Name, localState, globalState)

				result[idc.Name] = ema
			case "macd":
				_, _, hist := talib.Macd(src, idc.Params[0], idc.Params[1], idc.Params[2])
				reverse(hist)

				setIndicatorResult(hist, c.Name, idc.Name, localState, globalState)

				result[idc.Name] = hist
			default:
			}
		}

		results <- result

		for j := range c.Alerts {
			fired, n := runAlert(localState, c.Alerts[j])

			if fired {
				n.Source = c.Name
				notifications <- n
			}
		}
	}

	for j := range config.Alerts {
		fired, n := runAlert(globalState, config.Alerts[j])

		if fired {
			notifications <- n
		}
	}
}

func main() {
	if len(os.Args) >= 2 {
		loadConfig(os.Args[1])
	} else {
		loadConfig("config.json")
	}

	var state scriptState

	notifications := make(chan notification)
	results := make(chan dataset)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		mainLoop(state, notifications, results)

		for range ticker.C {
			mainLoop(state, notifications, results)
		}
	}()

	for {
		select {
		case n := <-notifications:
			fmt.Printf("Notification: %v\n", n)
		case r := <-results:
			fmt.Printf("Results: %v\n", r)
		}
	}
}
