package msghub

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type ID string

type Msg struct {
	From ID
	To   ID
	Data []string
}

func M(from, to ID, data []string) Msg {
	return Msg{From: from, To: to, Data: data}
}

const Spliter = "\x1f"

func Decode(raw []byte) (id ID, data []string) {
	rawdata := strings.Split(string(raw), Spliter)
	if len(rawdata) > 0 {
		id = ID(rawdata[0])
	}
	if len(rawdata) > 1 {
		data = rawdata[1:]
	}
	return
}

func Encode(id ID, data []string) []byte {
	if len(data) == 0 {
		return []byte(id)
	}
	return []byte(strings.Join([]string{string(id), strings.Join(data, Spliter)}, Spliter))
}

type Agent struct {
	conn *websocket.Conn

	C      chan Msg
	AuthUD interface{}
}

func NewAgent(conn *websocket.Conn, cache int) *Agent {
	if cache < 1 {
		cache = 1
	}
	return &Agent{
		conn:   conn,
		C:      make(chan Msg, cache),
		AuthUD: nil,
	}
}

type Service struct {
	Handle func(*Server, Msg)
}

type ServerFunc func()

type Server struct {
	C chan ServerFunc

	Agents   map[ID]*Agent
	Services map[ID]*Service

	// read-only
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	Upgrader         *websocket.Upgrader
	NewID            func() ID
	OnFailureSentMsg func(*Server, Msg)
	BeforeService    func(*Server, *Msg)
}

func NewServer() *Server {
	s := &Server{
		C:            make(chan ServerFunc, 256),
		Agents:       make(map[ID]*Agent),
		Services:     make(map[ID]*Service),
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
		Upgrader: &websocket.Upgrader{
			ReadBufferSize:  128,
			WriteBufferSize: 128,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		NewID: func() func() ID {
			id := 0
			return func() ID {
				id++
				return ID(strconv.Itoa(id))
			}
		}(),
		OnFailureSentMsg: func(s *Server, msg Msg) {
		},
		BeforeService: func(s *Server, msg *Msg) {},
	}
	go s.loop()
	return s
}

func (s *Server) loop() {
	for fn := range s.C {
		fn()
	}
}

func (s *Server) IsService(id ID) bool {
	return strings.HasPrefix(string(id), "@")
}

func (s *Server) Send(msg Msg) (ok bool) {
	defer func() {
		if !ok {
			s.OnFailureSentMsg(s, msg)
		}
	}()
	if s.IsService(msg.To) {
		s.BeforeService(s, &msg)
		service := s.Services[msg.To]
		if service == nil {
			return false
		}

		service.Handle(s, msg)
		return true
	} else { // is agent
		agent := s.Agents[msg.To]
		if agent == nil {
			return false
		}

		agent.C <- msg
		return true
	}
}

func timeZero() time.Time {
	var t time.Time
	return t
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	const MESSAGE_CACHE_SIZE = 100

	// add agent
	var (
		id    ID
		agent *Agent
	)
	ok := make(chan bool, 1)
	s.C <- func() {
		id = s.NewID()
		agent = NewAgent(conn, MESSAGE_CACHE_SIZE)
		s.Agents[id] = agent
		close(ok)
	}
	<-ok

	// remove agent
	defer func() {
		s.C <- func() {
			agent := s.Agents[id]
			if agent == nil {
				return
			}
			delete(s.Agents, id)

			agent.conn.Close()
			close(agent.C)
		}
	}()

	// write
	go func() {
		for msg := range agent.C {
			conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
			err := conn.WriteMessage(websocket.TextMessage, Encode(msg.From, msg.Data))
			if err != nil {
				conn.Close()
			}
			conn.SetWriteDeadline(timeZero())
		}
	}()

	// read
	for {
		conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
		_, p, err := conn.ReadMessage()
		if err != nil {
			return
		}

		to, data := Decode(p)
		s.C <- func() {
			s.Send(M(id, to, data))
		}
	}
}
