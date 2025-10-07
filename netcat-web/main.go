package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Netcat Noise Generator Webserver"))
	})

	http.HandleFunc("/ws/tail", tailWebSocketHandler)

	log.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func tailWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	filePath := "../netcat-server/priority1.log" // Change to your file path
	file, err := os.Open(filePath)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error opening file"))
		return
	}
	defer file.Close()

	// Seek to end of file
	file.Seek(0, os.SEEK_END)
	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')
		if err == nil {
			conn.WriteMessage(websocket.TextMessage, []byte(line))
		} else {
			time.Sleep(500 * time.Millisecond)
		}
	}
}