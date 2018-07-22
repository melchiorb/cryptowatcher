package main

import (
	"encoding/json"
	"log"
	"os"
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

	enc := json.NewEncoder(os.Stdout)
	enc.Encode(result)
}
