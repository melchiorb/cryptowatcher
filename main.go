package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	cc "./cryptocompare"
	ti "./tradeinterval"
	yaml "gopkg.in/yaml.v2"

	"github.com/Knetic/govaluate"
	talib "github.com/markcheno/go-talib"
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

type indicator struct {
	Name   string `json:"name" yaml:"name"`
	Type   string `json:"type" yaml:"type"`
	Params []int  `json:"params" yaml:"params"`
}

type watcher struct {
	Name string `json:"name" yaml:"name"`
	Type string `json:"type" yaml:"type"`
	Lua  string `json:"lua" yaml:"lua"`
	Expr string `json:"expr" yaml:"expr"`
}

type tradingpair struct {
	Name       string      `json:"name" yaml:"name"`
	Slug       string      `json:"slug" yaml:"slug"`
	Coin       string      `json:"coin" yaml:"coin"`
	Currency   string      `json:"currency" yaml:"currency"`
	Exchange   string      `json:"exchange" yaml:"exchange"`
	Interval   string      `json:"interval" yaml:"interval"`
	Length     int         `json:"length" yaml:"length"`
	Indicators []indicator `json:"indicators" yaml:"indicators"`
	Watchers   []watcher   `json:"watchers" yaml:"watchers"`
}

type notifier struct {
	Type      string `json:"type" yaml:"type"`
	Recipient string `json:"recipient" yaml:"recipient"`
	Sender    string `json:"sender" yaml:"sender"`
	Auth      string `json:"auth" yaml:"auth"`
	Format    string `json:"format" yaml:"format"`
}

type notification struct {
	Timestamp string `json:"timestamp" yaml:"timestamp"`
	Message   string `json:"message" yaml:"message"`
	Source    string `json:"source" yaml:"source"`
	Code      string `json:"code" yaml:"code"`
}

type timeseries = []float64
type dataset map[string]timeseries

type ohlcv5 [5]timeseries

type key struct {
	Tradingpair, Watcher string
}

var cache map[key]uint64

var config struct {
	Tradingpairs []tradingpair `json:"tradingpairs" yaml:"tradingpairs"`
	Watchers     []watcher     `json:"watchers" yaml:"watchers"`
	Notifiers    []notifier    `json:"notifiers" yaml:"notifiers"`
	Update       string        `json:"update" yaml:"update"`
	Verbose      bool          `json:"verbose" yaml:"verbose"`
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

func (state *scriptState) setBoth(name string, exprVal interface{}, luaVal interface{}) {
	state.setExpr(name, exprVal)
	state.setLua(name, luaVal)
}

func (n notification) format(template string) string {
	messages := map[string]string{
		"short":  "{message}",
		"normal": "{source}\n{message}",
		"long":   "{source}\n{message}\n{code}",
	}

	message := messages[template]

	message = strings.Replace(message, "{source}", n.Source, -1)
	message = strings.Replace(message, "{message}", n.Message, -1)
	message = strings.Replace(message, "{code}", n.Code, -1)

	return message
}

func sendNotification(n notification) {
	for i := range config.Notifiers {
		notif := config.Notifiers[i]

		switch notif.Type {
		case "telegram":
			notifyTelegram(n, notif)
		default:
			fmt.Println(n.format("short"))
		}
	}
}

func notifyTelegram(n notification, notif notifier) {
	message := url.QueryEscape(n.format(notif.Format))

	link := "https://api.telegram.org/bot{botId}:{apiKey}/sendMessage?chat_id={chatId}&text={text}"

	link = strings.Replace(link, "{botId}", notif.Sender, -1)
	link = strings.Replace(link, "{apiKey}", notif.Auth, -1)
	link = strings.Replace(link, "{chatId}", notif.Recipient, -1)
	link = strings.Replace(link, "{text}", message, -1)

	_, _ = http.Get(link)
}

func executeWatcher(state scriptState, watcher watcher) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = watcher.Name

	luaResult := false

	if watcher.Lua != "" {
		state.setLua("alert", func(v bool) { luaResult = v })

		err := state.lua.DoString(watcher.Lua)

		if err != nil {
			log.Fatal(err)
		}

		if luaResult {
			fired = true
			n.Code = watcher.Lua

			luaResult = false
		}
	} else if watcher.Expr != "" {
		exp, err := govaluate.NewEvaluableExpression(watcher.Expr)
		res, err := exp.Evaluate(state.expr)

		if err != nil {
			log.Fatal(err)
		}

		if res == true {
			fired = true
			n.Code = watcher.Expr
		}
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

		Interval := ti.Parse(t.Interval).MinHourDay()

		switch Interval.Unit {
		case ti.Minute:
			data = cc.Histominute(t.Coin, t.Currency, Interval.Num, t.Length, t.Exchange).Data
		case ti.Hour:
			data = cc.Histohour(t.Coin, t.Currency, Interval.Num, t.Length, t.Exchange).Data
		case ti.Day:
			data = cc.Histoday(t.Coin, t.Currency, Interval.Num, t.Length, t.Exchange).Data
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

		globalState.setAll(t.Slug+"_coin", t.Coin)
		globalState.setAll(t.Slug+"_currency", t.Currency)
		globalState.setAll(t.Slug+"_interval", t.Interval)
		globalState.setAll(t.Slug+"_length", t.Length)

		globalState.setBoth(t.Slug+"_open", rOpen[0], rOpen)
		globalState.setBoth(t.Slug+"_high", rHigh[0], rHigh)
		globalState.setBoth(t.Slug+"_low", rLow[0], rLow)
		globalState.setBoth(t.Slug+"_close", rClose[0], rClose)
		globalState.setBoth(t.Slug+"_vol", rVol[0], rVol)

		localState.setAll("coin", t.Coin)
		localState.setAll("currency", t.Currency)
		localState.setAll("interval", t.Interval)
		localState.setAll("length", t.Length)

		localState.setBoth("open", rOpen[0], rOpen)
		localState.setBoth("high", rHigh[0], rHigh)
		localState.setBoth("low", rLow[0], rLow)
		localState.setBoth("close", rClose[0], rClose)
		localState.setBoth("vol", rVol[0], rVol)

		for j := range t.Indicators {
			idc := t.Indicators[j]

			ohlcv := ohlcv5{open, high, low, close, vol}

			output := processIndicators(ohlcv, idc)
			rOutput := reverse(output)

			globalState.setExpr(t.Slug+"_"+idc.Name, rOutput[0])
			globalState.setLua(t.Slug+"_"+idc.Name, rOutput)

			localState.setExpr(idc.Name, rOutput[0])
			localState.setLua(idc.Name, rOutput)

			result[idc.Name] = output
		}

		results <- result

		for j := range t.Watchers {
			w := t.Watchers[j]
			fired, n := executeWatcher(localState, w)

			if fired {
				if cache[key{t.Slug, w.Name}] == 0 {
					n.Source = t.Name + " " + t.Interval
					notifications <- n
				}

				cache[key{t.Slug, w.Name}]++
			} else {
				cache[key{t.Slug, w.Name}] = 0
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
	defer configFile.Close()

	if err != nil {
		log.Fatal(err)
	}

	switch filepath.Ext(file) {
	case ".json":
		jsonParser := json.NewDecoder(configFile)
		err = jsonParser.Decode(&config)
	case ".yaml", ".yml":
		yamlParser := yaml.NewDecoder(configFile)
		err = yamlParser.Decode(&config)
	}

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

	update, err := time.ParseDuration(config.Update)

	if err != nil {
		update = time.Hour
	}

	ticker := time.NewTicker(update)
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
			go sendNotification(n)
		case r := <-results:
			if config.Verbose {
				fmt.Printf("Results: %v\n", r)
			}
		}
	}
}
