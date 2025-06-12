package main

import (
	"github.com/whoamixl/easyws"
	"log"
	"net/http"
	"time"
)

func main() {
	s := easyws.NewWebSocketServer()
	s.SetGenerateClientID(func(r *http.Request) string {
		return "test" + time.Time{}.String()
	})
	s.Hub.OnMessage = func(client *easyws.Client, messageType int, data []byte) error {
		log.Println(string(data))
		return nil
	}
	s.StartWithDefaults()
}
