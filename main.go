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

type watcher struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Code string `json:"code"`
}

type tradingpair struct {
	Name       string      `json:"name"`
	Coin       string      `json:"coin"`
	Currency   string      `json:"currency"`
	Exchange   string      `json:"exchange"`
	Indicators []indicator `json:"indicators"`
	Watchers   []watcher   `json:"watchers"`
}

type notification struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	Code      string `json:"code"`
}

type timeseries = []float64
type dataset map[string]timeseries

type ohlcv5 [5]timeseries

type key struct {
	Tradingpair, Watcher string
}

var cache map[key]uint64

var config struct {
	Tradingpairs []tradingpair `json:"tradingpairs"`
	Watchers     []watcher     `json:"watchers"`
	Scope        string        `json:"scope"`
	Length       int           `json:"length"`
	Verbose      bool          `json:"verbose"`
}

func reverse(numbers timeseries) timeseries {
	newNumbers := make(timeseries, len(numbers))
	for i, j := 0, len(numbers)-1; i < j; i, j = i+1, j-1 {
		newNumbers[i], newNumbers[j] = numbers[j], numbers[i]
	}
	return newNumbers
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

func executeWatcher(state scriptState, watcher watcher) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = watcher.Name
	n.Code = watcher.Code

	luaResult := false

	switch watcher.Type {
	case "lua":
		state.setLua("alert", func(v bool) { luaResult = v })

		err := state.lua.DoString(watcher.Code)

		if err != nil {
			log.Fatal(err)
		}

		if luaResult {
			fired = true
			luaResult = false
		}
	case "expr":
		exp, err := govaluate.NewEvaluableExpression(watcher.Code)
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

func processIndicators(src ohlcv5, idc indicator) timeseries {
	var result timeseries

	switch idc.Type {
	case "sma":
		result = talib.Sma(src[3], idc.Params[0])
	case "ema":
		result = talib.Ema(src[3], idc.Params[0])
	case "dema":
		result = talib.Dema(src[3], idc.Params[0])
	case "tema":
		result = talib.Tema(src[3], idc.Params[0])
	case "wma":
		result = talib.Wma(src[3], idc.Params[0])
	case "rsi":
		result = talib.Rsi(src[3], idc.Params[0])
	case "stochrsi":
		_, result = talib.StochRsi(src[3], idc.Params[0], idc.Params[1], idc.Params[2], talib.SMA)
	case "stoch":
		_, result = talib.Stoch(src[1], src[2], src[3], idc.Params[0], idc.Params[1], talib.SMA, idc.Params[2], talib.SMA)
	case "macd":
		_, _, result = talib.Macd(src[3], idc.Params[0], idc.Params[1], idc.Params[2])
	case "mom":
		result = talib.Mom(src[3], idc.Params[0])
	case "mfi":
		result = talib.Mfi(src[1], src[2], src[3], src[4], idc.Params[0])
	case "adx":
		result = talib.Adx(src[1], src[2], src[3], idc.Params[0])
	case "roc":
		result = talib.Roc(src[3], idc.Params[0])
	case "obv":
		result = talib.Obv(src[3], src[4])
	case "atr":
		result = talib.Atr(src[1], src[2], src[3], idc.Params[0])
	case "natr":
		result = talib.Natr(src[1], src[2], src[3], idc.Params[0])
	case "linearreg":
		result = talib.LinearReg(src[3], idc.Params[0])
	case "max":
		result = talib.Max(src[3], idc.Params[0])
	case "min":
		result = talib.Min(src[3], idc.Params[0])
	}

	return result
}

func mainLoop(notifications chan<- notification, results chan<- dataset) {
	var globalState scriptState
	globalState.init()
	defer globalState.close()

	for i := range config.Tradingpairs {
		t := config.Tradingpairs[i]

		result := make(dataset)

		var data []cc.Tick

		var localState scriptState
		localState.init()
		defer localState.close()

		if config.Scope == "hour" {
			data = cc.Histohour(t.Coin, t.Currency, config.Length, t.Exchange).Data
		} else {
			data = cc.Histoday(t.Coin, t.Currency, config.Length, t.Exchange).Data
		}

		open := cc.Open(data)
		high := cc.High(data)
		low := cc.Low(data)
		close := cc.Close(data)
		vol := cc.VolumeFrom(data)

		rOpen := reverse(open)
		rHigh := reverse(high)
		rLow := reverse(low)
		rClose := reverse(close)
		rVol := reverse(vol)

		result["open"] = open
		result["high"] = high
		result["low"] = low
		result["close"] = close
		result["vol"] = vol

		globalState.setAll(t.Name+"_coin", t.Coin)
		globalState.setAll(t.Name+"_currency", t.Currency)
		globalState.setAll("length", config.Length)

		localState.setAll("coin", t.Coin)
		localState.setAll("currency", t.Currency)
		localState.setAll("length", config.Length)

		globalState.setExpr(t.Name+"_open", rOpen[0])
		globalState.setExpr(t.Name+"_high", rHigh[0])
		globalState.setExpr(t.Name+"_low", rLow[0])
		globalState.setExpr(t.Name+"_close", rClose[0])
		globalState.setExpr(t.Name+"_vol", rVol[0])

		localState.setExpr("open", rOpen[0])
		localState.setExpr("high", rHigh[0])
		localState.setExpr("low", rLow[0])
		localState.setExpr("close", rClose[0])
		localState.setExpr("vol", rVol[0])

		globalState.setLua(t.Name+"_open", rOpen)
		globalState.setLua(t.Name+"_high", rHigh)
		globalState.setLua(t.Name+"_low", rLow)
		globalState.setLua(t.Name+"_close", rClose)
		globalState.setLua(t.Name+"_vol", rVol)

		localState.setLua("open", rOpen)
		localState.setLua("high", rHigh)
		localState.setLua("low", rLow)
		localState.setLua("close", rClose)
		localState.setLua("vol", rVol)

		for j := range t.Indicators {
			idc := t.Indicators[j]

			ohlcv := ohlcv5{open, high, low, close, vol}

			output := processIndicators(ohlcv, idc)
			rOutput := reverse(output)

			globalState.setExpr(t.Name+"_"+idc.Name, rOutput[0])
			globalState.setLua(t.Name+"_"+idc.Name, rOutput)

			localState.setExpr(idc.Name, rOutput[0])
			localState.setLua(idc.Name, rOutput)

			result[idc.Name] = output
		}

		results <- result

		for j := range t.Watchers {
			w := t.Watchers[j]
			fired, n := executeWatcher(localState, w)

			if fired {
				if cache[key{t.Name, w.Name}] == 0 {
					n.Source = t.Name
					notifications <- n
				}

				cache[key{t.Name, w.Name}]++
			} else {
				cache[key{t.Name, w.Name}] = 0
			}
		}
	}

	for j := range config.Watchers {
		w := config.Watchers[j]

		fired, n := executeWatcher(globalState, w)

		if fired {
			if cache[key{"global", w.Name}] == 0 {
				notifications <- n
			}

			cache[key{"global", w.Name}]++
		} else {
			cache[key{"global", w.Name}] = 0
		}
	}
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

func main() {
	if len(os.Args) >= 2 {
		loadConfig(os.Args[1])
	} else {
		loadConfig("config.json")
	}

	cache = make(map[key]uint64)

	notifications := make(chan notification)
	results := make(chan dataset)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		mainLoop(notifications, results)

		for range ticker.C {
			mainLoop(notifications, results)
		}
	}()

	for {
		select {
		case n := <-notifications:
			fmt.Printf("Notification: %v\n", n)
		case r := <-results:
			if config.Verbose {
				fmt.Printf("Results: %v\n", r)
			}
		}
	}
}
