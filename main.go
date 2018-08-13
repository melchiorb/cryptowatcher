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
	ss "./scriptstate"
	ti "./tradeinterval"
	yaml "gopkg.in/yaml.v2"

	talib "github.com/markcheno/go-talib"
)

type indicator struct {
	Name   string `json:"name" yaml:"name"`
	Type   string `json:"type" yaml:"type"`
	Params []int  `json:"params" yaml:"params"`
}

type watcher struct {
	Name   string   `json:"name" yaml:"name"`
	Lua    string   `json:"lua" yaml:"lua"`
	Expr   string   `json:"expr" yaml:"expr"`
	Values []string `json:"values" yaml:"values"`
}

type tradingpair struct {
	Name       string      `json:"name" yaml:"name"`
	Slug       string      `json:"slug" yaml:"slug"`
	Coin       string      `json:"coin" yaml:"coin"`
	Currency   string      `json:"currency" yaml:"currency"`
	Exchange   string      `json:"exchange" yaml:"exchange"`
	Interval   string      `json:"interval" yaml:"interval"`
	Length     int         `json:"length" yaml:"length"`
	Update     []string    `json:"update" yaml:"update"`
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
	Timestamp string             `json:"timestamp" yaml:"timestamp"`
	Message   string             `json:"message" yaml:"message"`
	Source    string             `json:"source" yaml:"source"`
	Code      string             `json:"code" yaml:"code"`
	Values    map[string]float64 `json:"values" yaml:"values"`
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

func (n notification) format(template string) string {
	message, values := "", ""

	for k, v := range n.Values {
		if v < 1 {
			// 0 digits
			values += fmt.Sprintf("%-12s: %-8.6f\n", strings.Title(k), v)
		} else if v < 1000 {
			// three digits
			values += fmt.Sprintf("%-12s: %-8.2f\n", strings.Title(k), v)
		} else {
			// four digits
			values += fmt.Sprintf("%-12s: %-8.0f\n", strings.Title(k), v)
		}
	}

	if n.Source != "" {
		message += n.Source + "\n"
	}

	message += n.Message + "\n"

	switch template {
	case "normal":
		if values != "" {
			message += "\n" + values
		}
	case "long":
		if values != "" {
			message += "\n" + values
		}

		message += "\n" + n.Code
	default:
	}

	return message
}

func sendNotification(n notification) {
	for _, notif := range config.Notifiers {
		switch notif.Type {
		case "telegram":
			notifyTelegram(n, notif)
		default:
			fmt.Printf("%v---\n", n.format("normal"))
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

func executeWatcher(state ss.State, watcher watcher) (bool, notification) {
	fired := false

	var n notification
	n.Timestamp = time.Now().Format(time.RFC850)
	n.Message = watcher.Name

	if watcher.Lua != "" {
		luaResult := false
		state.SetLua("alert", func(v bool) { luaResult = v })

		err := state.EvalLua(watcher.Lua)

		if err != nil {
			log.Fatal(err)
		}

		if luaResult {
			fired = true
			n.Code = watcher.Lua
		}
	} else if watcher.Expr != "" {
		res, err := state.EvalExpr(watcher.Expr)

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

func processIndicators(src ohlcv5, idc indicator) ([]timeseries, []string) {
	var result []timeseries
	var labels []string

	switch idc.Type {
	case "sma":
		r1 := talib.Sma(src[3], idc.Params[0])
		result = append(result, r1)
		labels = append(labels, "")
	case "ema":
		r1 := talib.Ema(src[3], idc.Params[0])
		result = append(result, r1)
		labels = append(labels, "")
	case "dema":
		r1 := talib.Dema(src[3], idc.Params[0])
		result = append(result, r1)
		labels = append(labels, "")
	case "tema":
		r1 := talib.Tema(src[3], idc.Params[0])
		result = append(result, r1)
		labels = append(labels, "")
	case "wma":
		r1 := talib.Wma(src[3], idc.Params[0])
		result = append(result, r1)
		labels = append(labels, "")
	case "rsi":
		r1 := talib.Rsi(src[3], idc.Params[0])
		result = append(result, r1)
		labels = append(labels, "")
	case "stochrsi":
		r1, r2 := talib.StochRsi(src[3], idc.Params[0], idc.Params[1], idc.Params[2], talib.SMA)
		result = append(result, r1, r2)
		labels = append(labels, "_K", "_D")
	case "stoch":
		r1, r2 := talib.Stoch(src[1], src[2], src[3], idc.Params[0], idc.Params[1], talib.SMA, idc.Params[2], talib.SMA)
		result = append(result, r1, r2)
		labels = append(labels, "_K", "_D")
	case "macd":
		r1, r2, r3 := talib.Macd(src[3], idc.Params[0], idc.Params[1], idc.Params[2])
		result = append(result, r1, r2, r3)
		labels = append(labels, "", "_Sig", "_Hist")
	case "mom":
		r1 := talib.Mom(src[3], idc.Params[0])
		result = append(result, r1)
	case "mfi":
		r1 := talib.Mfi(src[1], src[2], src[3], src[4], idc.Params[0])
		result = append(result, r1)
	case "adx":
		r1 := talib.Adx(src[1], src[2], src[3], idc.Params[0])
		result = append(result, r1)
	case "roc":
		r1 := talib.Roc(src[3], idc.Params[0])
		result = append(result, r1)
	case "obv":
		r1 := talib.Obv(src[3], src[4])
		result = append(result, r1)
	case "atr":
		r1 := talib.Atr(src[1], src[2], src[3], idc.Params[0])
		result = append(result, r1)
	case "natr":
		r1 := talib.Natr(src[1], src[2], src[3], idc.Params[0])
		result = append(result, r1)
	case "linearreg":
		r1 := talib.LinearReg(src[3], idc.Params[0])
		result = append(result, r1)
	case "max":
		r1 := talib.Max(src[3], idc.Params[0])
		result = append(result, r1)
	case "min":
		r1 := talib.Min(src[3], idc.Params[0])
		result = append(result, r1)
	}

	return result, labels
}

func mainLoop(notifications chan<- notification, results chan<- dataset) {
	// global results
	globalResults := make(dataset)

	// global script state
	var globalState ss.State
	globalState.Init()
	defer globalState.Close()

	for _, t := range config.Tradingpairs {
		// local results
		localResults := make(dataset)

		// time series data
		var data []cc.Tick

		// local script state
		var localState ss.State
		localState.Init()
		defer localState.Close()

		// calculate time frame
		Interval := ti.Parse(t.Interval).MinHourDay()

		// load time series data
		switch Interval.Unit {
		case ti.Minute:
			data = cc.Histominute(t.Coin, t.Currency, Interval.Num, t.Length, t.Exchange).Data
		case ti.Hour:
			data = cc.Histohour(t.Coin, t.Currency, Interval.Num, t.Length, t.Exchange).Data
		case ti.Day:
			data = cc.Histoday(t.Coin, t.Currency, Interval.Num, t.Length, t.Exchange).Data
		}

		// get time series data
		open := cc.Open(data)
		high := cc.High(data)
		low := cc.Low(data)
		close := cc.Close(data)
		vol := cc.VolumeFrom(data)

		// reverse time series data for scripts
		rOpen := reverse(open)
		rHigh := reverse(high)
		rLow := reverse(low)
		rClose := reverse(close)
		rVol := reverse(vol)

		// set global result data
		globalResults[t.Slug+"_open"] = open
		globalResults[t.Slug+"_high"] = high
		globalResults[t.Slug+"_low"] = low
		globalResults[t.Slug+"_close"] = close
		globalResults[t.Slug+"_vol"] = vol

		// set local result data
		localResults["open"] = open
		localResults["high"] = high
		localResults["low"] = low
		localResults["close"] = close
		localResults["vol"] = vol

		// set global state
		globalState.SetAll(t.Slug+"_coin", t.Coin)
		globalState.SetAll(t.Slug+"_currency", t.Currency)
		globalState.SetAll(t.Slug+"_interval", t.Interval)
		globalState.SetAll(t.Slug+"_length", t.Length)

		globalState.SetBoth(t.Slug+"_open", rOpen[0], rOpen)
		globalState.SetBoth(t.Slug+"_high", rHigh[0], rHigh)
		globalState.SetBoth(t.Slug+"_low", rLow[0], rLow)
		globalState.SetBoth(t.Slug+"_close", rClose[0], rClose)
		globalState.SetBoth(t.Slug+"_vol", rVol[0], rVol)

		// set local state
		localState.SetAll("coin", t.Coin)
		localState.SetAll("currency", t.Currency)
		localState.SetAll("interval", t.Interval)
		localState.SetAll("length", t.Length)

		localState.SetBoth("open", rOpen[0], rOpen)
		localState.SetBoth("high", rHigh[0], rHigh)
		localState.SetBoth("low", rLow[0], rLow)
		localState.SetBoth("close", rClose[0], rClose)
		localState.SetBoth("vol", rVol[0], rVol)

		// process indicators
		for _, idc := range t.Indicators {
			// create input data
			ohlcv := ohlcv5{open, high, low, close, vol}

			outputs, labels := processIndicators(ohlcv, idc)

			// process indicator
			for i, output := range outputs {
				rOutput := reverse(output)
				label := labels[i]

				// add indicator output to state
				globalState.SetExpr(t.Slug+"_"+idc.Name+label, rOutput[0])
				globalState.SetLua(t.Slug+"_"+idc.Name+label, rOutput)

				localState.SetExpr(idc.Name+label, rOutput[0])
				localState.SetLua(idc.Name+label, rOutput)

				globalResults[t.Slug+"_"+idc.Name+label] = output
				localResults[idc.Name+label] = output
			}
		}

		// send update
		if len(t.Update) > 0 {
			var n notification
			n.Timestamp = time.Now().Format(time.RFC850)
			n.Message = "Update"

			n.Source = t.Name + " " + t.Interval
			n.Values = make(map[string]float64)

			for _, key := range t.Update {
				results := localResults[key]
				n.Values[key] = results[len(results)-1]
			}

			notifications <- n
		}

		// execute time series watchers
		for _, w := range t.Watchers {
			// execute watcher
			fired, n := executeWatcher(localState, w)

			// process watcher result
			if fired {
				// check for previous notification
				if cache[key{t.Slug, w.Name}] == 0 {
					// set return values
					n.Source = t.Name + " " + t.Interval
					n.Values = make(map[string]float64)

					for _, key := range w.Values {
						results := localResults[key]
						n.Values[key] = results[len(results)-1]
					}

					// send notification
					notifications <- n
				}

				// increase notification counter
				cache[key{t.Slug, w.Name}]++
			} else {
				// reset cache
				cache[key{t.Slug, w.Name}] = 0
			}
		}
	}

	// execute global watchers
	for _, w := range config.Watchers {
		// execure watcher
		fired, n := executeWatcher(globalState, w)

		// process watcher result
		if fired {
			// check for previous notification
			if cache[key{"global", w.Name}] == 0 {
				// set return values
				n.Values = make(map[string]float64)

				for _, key := range w.Values {
					results := globalResults[key]
					n.Values[key] = results[len(results)-1]
				}

				// send notification
				notifications <- n
			}

			// increase notification counter
			cache[key{"global", w.Name}]++
		} else {
			// reset cache
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
	// load config from argument or default
	if len(os.Args) >= 2 {
		loadConfig(os.Args[1])
	} else {
		loadConfig("config.yaml")
	}

	// notification cache
	cache = make(map[key]uint64)

	// notification and result channels
	notifications := make(chan notification)
	results := make(chan dataset)

	// update interval
	update, err := time.ParseDuration(config.Update)

	if err != nil {
		update = time.Hour
	}

	// main loop ticker
	ticker := time.NewTicker(update)
	defer ticker.Stop()

	// main loop
	go func() {
		mainLoop(notifications, results)

		for range ticker.C {
			mainLoop(notifications, results)
		}
	}()

	// process notifictions and results
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
