package veneur

import (
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second
)

var (
	clientChan chan []byte
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
)

func serveWs(w http.ResponseWriter, r *http.Request) {
	log.Debug("WS serving")
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Error(err)
		}
		return
	}

	go writer(ws)
}

func writer(ws *websocket.Conn) {
	// Not doing any type of ping/pong for keeping things alive here, as we're going
	// to be sending data pretty often.
	for {
		select {
		case m := <-clientChan:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.TextMessage, []byte(m)); err != nil {
				return
			}
		}
	}
}

// BeginHTTP initiates the HTTP/websocket side of Veneur
func BeginHTTP(channel chan []byte) {
	log.WithFields(log.Fields{
		"addr": Config.WebsocketAddr,
	}).Info("Starting websocket")
	http.HandleFunc("/ws", serveWs)
	go func() {
		if err := http.ListenAndServe(Config.WebsocketAddr, nil); err != nil {
			log.Fatal(err)
		}
	}()
}
