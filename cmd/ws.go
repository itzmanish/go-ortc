package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/itzmanish/go-ortc/pkg/rtc"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type MessageType uint8

const (
	JoinRoomMessage MessageType = iota
	LeaveRoomMessage
	GetRouterCapability
	CreateTransport
	CloseTransport
	ConnectTransport
	Produce
	ProduceData
	ProducerToggle
	CloseProducer
	Consume
	ConsumeData
	ConsumerResume
	ConsumerPause

	PeerJoinedBroadcast = 15

	Error = 25
)

type WebSocketMessage struct {
	Id          uint `json:"id"`
	MessageType `json:"method"`
	IsError     bool   `json:"isError"`
	IsBroadcast bool   `json:"isBroadcast"`
	Payload     string `json:"payload"`
}

type WebSocketErrorMessage struct {
	Message string `json:"error"`
}

type WebSocketHandlerFn func(*Peer, *WebSocketMessage, func([]byte, error))

type WSServer struct {
	log   logger.Logger
	rooms map[uint]*Room
	sfu   *rtc.SFU
}

func NewWSServer() *WSServer {
	sfu, err := rtc.NewSFU()
	if err != nil {
		log.Fatal("Unable to create SFU", err)
	}
	return &WSServer{
		log:   logger.NewLogger("Websocket Server"),
		rooms: make(map[uint]*Room),
		sfu:   sfu,
	}
}

func (s WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	roomId := params.Get("roomId")
	peerId := params.Get("peerId")
	if len(roomId) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Room ID is not provided"))
		return
	}
	if len(peerId) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Peer ID is not provided"))
		return
	}
	rId, err := strconv.Atoi(roomId)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Room ID is not a valid integer"))
		return
	}

	pId, err := strconv.Atoi(peerId)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Peer ID is not a valid integer"))
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.log.Error(err)
		return
	}
	defer c.Close(websocket.StatusInternalError, "Closing socket connection")

	room := s.CreateOrGetRoom(uint(rId))
	peer := room.AddPeer(uint(pId))
	for {
		err = s.handle(r.Context(), c, room, peer)
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			return
		}
		if err != nil {
			s.log.Errorf("failed to send response with %v: %v", r.RemoteAddr, err)
			return
		}
	}
}

func (s WSServer) handle(ctx context.Context, conn *websocket.Conn, room *Room, peer *Peer) error {
	var data WebSocketMessage

	err := wsjson.Read(context.Background(), conn, &data)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	cb := func(res []byte, err error) {
		if err != nil {
			s.log.Error("Error on callback:", err)
			db, err := json.Marshal(WebSocketErrorMessage{
				Message: err.Error(),
			})
			if err != nil {
				s.log.Error("Error on marshing error message for sending to client", err)
				return
			}
			data.Payload = string(db)
			data.IsError = true
		}
		if res != nil {
			data.Payload = string(res)
		}
		err = wsjson.Write(ctx, conn, data)
		errCh <- err
	}
	room.HandleIncomingMessage(peer, &data, cb)
	data.Payload = ""
	return <-errCh
}

func (s *WSServer) CreateOrGetRoom(rId uint) *Room {
	room, ok := s.rooms[rId]
	if !ok {
		room = NewRoom(rId, s.sfu.NewRouter())
		s.rooms[rId] = room
	}
	return room
}

func BuildMessage(msg any) ([]byte, error) {
	return json.Marshal(msg)
}
