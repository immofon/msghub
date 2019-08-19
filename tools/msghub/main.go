package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/immofon/msghub"
)

func main() {
	cmd := os.Getenv("cmd")
	switch cmd {
	case "daemon":
		daemon()
	case "client":
		client()
	default:
		fmt.Println("help: ")
		fmt.Println("Please set env: cmd={daemon|client}")
	}
}

func daemon() {
	s := msghub.NewServer()
	s.OnConnect = func(s *msghub.Server, id msghub.ID) {
		s.Send(msghub.M("@whoami", id, string(id)))
	}
	s.OnDisconnect = func(s *msghub.Server, id msghub.ID) {
		for to, _ := range s.Agents {
			s.Send(msghub.M("#disconnected", to, string(id)))
		}
	}

	log.Print("listen on: ", ":9817")
	err := http.ListenAndServe(":9817", s)
	if err != nil {
		panic(err)
	}
}

func client() {
	c := msghub.NewClient("ws://localhost:9817")
	c.On = func(msg msghub.Msg) {
		fmt.Println(msg.From, msg.Data)
	}

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			raw := []byte(strings.Replace(scanner.Text(), " ", msghub.Spliter, -1))
			to, data := msghub.Decode(raw)
			c.C <- msghub.M("", to, data...)
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading standard input:", err)
		}

	}()

	for {
		log.Print("try to connect")
		c.Dial()
	}

}
