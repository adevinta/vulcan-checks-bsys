package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
)

type Message struct {
	ExternalID string    `json:"external_id"`
	Received   time.Time `json:"Received"`
	Payload    string    `json:"payload"`
}

// SimpleMQ is a very simple queue.
type SimpleMQ struct {
	Addr           string
	Path           string
	Queue          []Message
	Srv            *http.Server
	serverFinished chan error
	sync.Mutex
}

// New creates a new simple queue service that will listen to the given address.
func New(addr, path string) *SimpleMQ {
	return &SimpleMQ{
		Path:           path,
		Addr:           addr,
		Queue:          make([]Message, 0, 100),
		serverFinished: make(chan error, 1),
	}
}

// Enqueue enqueue a message to a queue.
func (s *SimpleMQ) Enqueue(m Message) {
	s.Lock()
	defer s.Unlock()
	s.Queue = append(s.Queue, m)
}

// Dequeue dequeue an element from the given queue.
func (s *SimpleMQ) Dequeue() Message {
	s.Lock()
	defer s.Unlock()
	msgs := s.Queue
	if len(msgs) < 1 {
		return Message{}
	}
	m := msgs[0]
	msgs = msgs[1:]
	s.Queue = msgs
	return m
}

func (s *SimpleMQ) handlePATCHMessage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("external_id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error queue_id is mandatory"))
		return
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(err.Error()))
		return
	}

	if string(payload) == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte("body can not be empty"))
		return
	}

	// We block the request here to backpresure the consumer if needed.
	m := Message{
		ExternalID: id,
		Payload:    string(payload),
		Received:   time.Now(),
	}
	s.Enqueue(m)
	w.WriteHeader(http.StatusOK)
}

func (s *SimpleMQ) handleGETMessage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	m := s.Dequeue()
	if m.Payload == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	e := json.NewEncoder(w)
	err := e.Encode(m)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

func (s *SimpleMQ) handleNotFoundRoute(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("request received: %+v\n", r.URL)
	w.WriteHeader(http.StatusForbidden)
}

// ListenAndServe starts de queue, it blocks the calling goroutine.
func (s *SimpleMQ) ListenAndServe() error {
	r := httprouter.New()
	path := s.Path
	if path != "" {
		path = "/" + path
	}
	r.NotFound = http.HandlerFunc(s.handleNotFoundRoute)
	route := fmt.Sprintf("%s/:external_id", path)
	r.PATCH(route, s.handlePATCHMessage)
	r.GET(route, s.handleGETMessage)
	server := &http.Server{Addr: s.Addr, Handler: r}
	s.Srv = server
	return server.ListenAndServe()

}

// Stop stops the underlaying http server.
func (s *SimpleMQ) Stop() error {
	ctx := context.Background()
	return s.Srv.Shutdown(ctx)
}
