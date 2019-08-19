package msghub

import (
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	URL string

	C            chan Msg
	On           func(Msg)
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func NewClient(url string) *Client {
	return &Client{
		URL:          url,
		C:            make(chan Msg, 100),
		On:           func(msg Msg) {},
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
	}
}

func (c *Client) Dial() {
	conn, _, err := websocket.DefaultDialer.Dial(c.URL, nil)
	if err != nil {
		return
	}

	// write
	go func() {
		for msg := range c.C {
			conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
			err := conn.WriteMessage(websocket.TextMessage, Encode(msg.To, msg.Data))
			if err != nil {
				conn.Close()
				return
			}
			conn.SetWriteDeadline(timeZero())
		}
	}()

	// read
	for {
		conn.SetReadDeadline(time.Now().Add(c.ReadTimeout))
		_, p, err := conn.ReadMessage()
		if err != nil {
			return
		}

		from, data := Decode(p)
		c.On(M(from, "", data...))
	}
}
