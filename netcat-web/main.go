package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/hpcloud/tail"
	_ "github.com/mattn/go-sqlite3"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	http.HandleFunc("/", getHomePage)
	http.HandleFunc("/terminal", getTerminalScreen)

	http.HandleFunc("/ws/tail", tailWebSocketHandler)
	http.HandleFunc("/connections", getConnections)
	http.HandleFunc("/connection-details", getConnectionDetails)
	http.HandleFunc("/update-connection", setConnectionDetails)

	log.Println("Server started at :8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

func getHomePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	http.ServeFile(w, r, "index.html")
}

func getTerminalScreen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	http.ServeFile(w, r, "terminal.html")
}

func setConnectionDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Handle saving connection details
	var req struct {
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Nickname string `json:"nickname"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	ip := req.IP
	port := req.Port
	nickname := req.Nickname

	if ip == "" || port == "" {
		http.Error(w, "Missing ip or port parameter", http.StatusBadRequest)
		return
	}

	dbPath := "../netcat-server/commands.db"
	db, err := sql.Open("sqlite3", dbPath)

	if err != nil {
		log.Fatalf("Error opening DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("UPDATE connections SET nickname = ? WHERE ip = ? AND port = ?", nickname, ip, port)
	if err != nil {
		log.Printf("Failed to update connection: %v", err)
		http.Error(w, "Failed to update connection", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection updated successfully"))
}

func getConnectionDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := r.URL.Query().Get("ip")
	port := r.URL.Query().Get("port")
	if ip == "" || port == "" {
		http.Error(w, "Missing ip or port parameter", http.StatusBadRequest)
		return
	}

	dbPath := "../netcat-server/commands.db"
	db, err := sql.Open("sqlite3", dbPath)

	if err != nil {
		log.Fatalf("Error opening DB: %v", err)
	}
	defer db.Close()

	var nickNull sql.NullString
	var nickname, uname, user string
	err = db.QueryRow("SELECT nickname, uname, user FROM connections WHERE ip = ? AND port = ?", ip, port).
		Scan(&nickNull, &uname, &user)
	if err != nil {
		log.Printf("Failed to retrieve connection details: %v", err)
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}
	if nickNull.Valid {
		nickname = nickNull.String
	} else {
		nickname = ""
	}
	response := map[string]interface{}{
		"nickname": nickname,
		"hostname": strings.Split(uname, " ")[1], // Extract hostname from uname
		"os":       uname,
		"user":     user,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func getConnections(w http.ResponseWriter, r *http.Request) {
	dbPath := "../netcat-server/commands.db"
	db, err := sql.Open("sqlite3", dbPath)

	if err != nil {
		log.Fatalf("Error opening DB: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT ip, port, nickname FROM connections")
	if err != nil {
		http.Error(w, "Failed to query connections", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Connection struct {
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Nickname string `json:"nickname"`
	}

	var connections []Connection
	for rows.Next() {
		var conn Connection
		var nick sql.NullString
		if err := rows.Scan(&conn.IP, &conn.Port, &nick); err != nil {
			http.Error(w, "Failed to scan connection", http.StatusInternalServerError)
			return
		}
		if nick.Valid {
			conn.Nickname = nick.String
		} else {
			conn.Nickname = ""
		}
		connections = append(connections, conn)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(connections); err != nil {
		http.Error(w, "Failed to encode connections", http.StatusInternalServerError)
		return
	}
}

func tailWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP request to a WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	// The file you want to tail
	// filePath := "../netcat-server/priority1.log" // change to your file path
	fileDir := "../netcat-server/command_output"
	ip := r.URL.Query().Get("ip")
	port := r.URL.Query().Get("port")
	if ip == "" || port == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("Missing ip or port parameter"))
		return
	}
	filePath := fileDir + "/" + ip + "_" + port + "_command.log"

	// Ensure the log directory exists
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		log.Println("Error creating log directory:", err)
		return
	}
	// Ensure the log file exists
	f, err := os.OpenFile(filePath, os.O_CREATE, 0644)
	if err != nil {
		log.Println("Error creating log file:", err)
		return
	}
	f.Close()

	// Start tailing the file (follows new lines)
	t, err := tail.TailFile(filePath, tail.Config{
		Follow:    true,
		ReOpen:    true,
		MustExist: true,
		Poll:      true,
	})
	if err != nil {
		log.Println("Error tailing file:", err)
		return
	}
	defer t.Cleanup()

	// Channel for incoming messages from client
	clientMsgChan := make(chan string)

	// Goroutine to read messages from WebSocket
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("WebSocket read error:", err)
				close(clientMsgChan)
				return
			}
			clientMsgChan <- string(msg)
		}
	}()

	// Listen for new lines and send over WebSocket
	for {
		select {
		case line, ok := <-t.Lines:
			if !ok {
				return
			}
			if line.Err != nil {
				log.Println("Tail error:", line.Err)
				continue
			}
			err = conn.WriteMessage(websocket.TextMessage, []byte(line.Text))
			if err != nil {
				log.Println("WebSocket write error:", err)
				return
			}
		case msg, ok := <-clientMsgChan:
			if !ok {
				return
			}
			// Call your function with the received message
			handleClientMessage(msg)
		}
	}
}

// Example function to handle received client messages
func handleClientMessage(msg string) {
	log.Printf("Received command from client: %s", msg)
	dbPath := "../netcat-server/commands.db"
	db, err := sql.Open("sqlite3", dbPath)

	if err != nil {
		log.Fatalf("Error opening DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO priority_commands (command) VALUES (?)", msg)
	if err != nil {
		log.Printf("Failed to add priority command: %v", err)
	}
}
