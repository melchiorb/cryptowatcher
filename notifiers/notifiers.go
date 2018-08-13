package notifiers

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Notifier provides an interface to different notifiers
type Notifier interface {
	Init(ID string, Key string)
	Send(chatID string, text string)
}

// Telegram holds authentication state for telegram messaging
type Telegram struct {
	BotID  string
	APIKey string
}

// Init sets the authentication parameters for telegram
func (t Telegram) Init(ID string, Key string) {
	t.BotID = ID
	t.APIKey = Key
}

// Send sends a message to a recipient
func (t Telegram) Send(chatID string, text string) {
	chatID = url.QueryEscape(chatID)
	text = url.QueryEscape(text)

	link := "https://api.telegram.org/bot{botID}:{apiKey}/sendMessage?chat_id={chatID}&text={text}"

	link = strings.Replace(link, "{botID}", t.BotID, -1)
	link = strings.Replace(link, "{apiKey}", t.APIKey, -1)
	link = strings.Replace(link, "{chatID}", chatID, -1)
	link = strings.Replace(link, "{text}", text, -1)

	_, err := http.Get(link)

	if err != nil {
		log.Fatal(err)
	}
}
