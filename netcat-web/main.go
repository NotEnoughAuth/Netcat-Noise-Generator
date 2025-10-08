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
		tmpl := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Netcat Noise Generator Webserver</title>
		</head>
		<body>
			<h1>Netcat Noise Generator Webserver</h1>
			<p>Welcome to the Netcat Noise Generator!</p>
			<button onclick="connectWS()">Connect to WebSocket</button>
			<pre id="output"></pre>
			<script>
				function connectWS() {
					const ws = new WebSocket("ws://localhost:8080/ws/tail");
					ws.onmessage = function(event) {
						document.getElementById("output").textContent += event.data;
					};
				}
			</script>
		</body>
		</html>
		`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(tmpl))
	})

	http.HandleFunc("/ws/tail", tailWebSocketHandler)

	log.Println("Server started at :8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
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