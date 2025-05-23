package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/octoper/go-ray"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
	} `yaml:"production"`

	Archive struct {
		CheckInterval   int    `yaml:"check_interval"`
		SourceStatus    string `yaml:"source_status"`
		ArchivingStatus string `yaml:"archiving_status"`
		ArchivedStatus  string `yaml:"archived_status"`
		ErrorStatus     string `yaml:"error_status"`
		ArchiveURL      string `yaml:"archive_url"`
	} `yaml:"archive"`

	SMTP struct {
		Server   string `yaml:"server"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		From     string `yaml:"from"`
		To       string `yaml:"to"`
	} `yaml:"smtp"`
}

type ArchiveResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Result string `json:"result"`
		File   string `json:"file"`
	} `json:"result"`
}

var (
	config Config
	db     *sql.DB
)

func main() {
	initConfig("config.yml")
	initDB()
	defer db.Close()

	ticker := time.NewTicker(time.Duration(config.Archive.CheckInterval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processPotentialDeals()
	}
}

func initConfig(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	if err = yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}
}

func initDB() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		config.Database.User,
		config.Database.Password,
		config.Database.Host,
		config.Database.Port,
		config.Database.Name)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatalf("Error pinging database: %v", err)
	}
}

func processPotentialDeals() {
	rows, err := db.Query(
		"SELECT potentialid FROM vtiger_potential WHERE archive_status = ?",
		config.Archive.SourceStatus,
	)
	if err != nil {
		log.Printf("Error querying potential deals: %v", err)
		return
	}
	defer rows.Close()

	var potentialIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			log.Printf("Error scanning potentialid: %v", err)
			continue
		}
		potentialIDs = append(potentialIDs, id)
	}

	for _, id := range potentialIDs {
		go processDeal(id)
	}
}

func processDeal(dealID string) {
	if err := updateDealStatus(dealID, config.Archive.ArchivingStatus); err != nil {
		log.Printf("Error updating status to Archiving: %v", err)
		return
	}

	resp, err := http.Get(fmt.Sprintf("%s?deal=%s", config.Archive.ArchiveURL, dealID))
	if err != nil {
		handleError(dealID, fmt.Errorf("archive request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	var archiveResp ArchiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&archiveResp); err != nil {
		handleError(dealID, fmt.Errorf("failed to decode response: %v", err))
		return
	}

	if archiveResp.Success {
		if err := updateDealStatus(dealID, config.Archive.ArchivedStatus); err != nil {
			log.Printf("Error updating status to Archived: %v", err)
		}
		sendEmail("Deal Archived Successfully",
			fmt.Sprintf("Deal ID: %s\nArchive File: %s", dealID, archiveResp.Result.File))
	} else {
		handleError(dealID, fmt.Errorf("archive failed: %s", archiveResp.Result.Result))
	}
}

func handleError(dealID string, err error) {
	log.Printf("Deal %s error: %v", dealID, err)
	if updateErr := updateDealStatus(dealID, config.Archive.ErrorStatus); updateErr != nil {
		log.Printf("Error updating status to Error: %v", updateErr)
	}
	sendEmail("Deal Archive Error",
		fmt.Sprintf("Deal ID: %s\nError: %v", dealID, err))
}

func updateDealStatus(dealID, status string) error {
	_, err := db.Exec(
		"UPDATE vtiger_potential SET archive_status = ? WHERE potentialid = ?",
		status, dealID,
	)
	return err
}

func sendEmail(subject, body string) {
	// 1. Устанавливаем TCP-соединение
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", config.SMTP.Server, config.SMTP.Port))
	if err != nil {
		log.Printf("SMTP Connection Error: %v", err)
		return
	}
	defer conn.Close()

	// 2. Создаем SMTP-клиент
	client, err := smtp.NewClient(conn, config.SMTP.Server)
	if err != nil {
		log.Printf("Error creating client: %v", err)
		return
	}
	defer client.Close()

	// 3. Обязательный EHLO перед STARTTLS
	if err := client.Hello("localhost"); err != nil {
		log.Printf("Error EHLO: %v", err)
		return
	}

	// 4. Активируем STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName:         config.SMTP.Server,
			InsecureSkipVerify: false,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			log.Printf("Error STARTTLS: %v", err)
			return
		}
	}

	// 5. Реализация AUTH LOGIN вместо PLAIN
	auth := &loginAuth{
		username: config.SMTP.Username,
		password: config.SMTP.Password,
	}

	if err := client.Auth(auth); err != nil {
		log.Printf("Auth Error: %v", err)
		return
	}

	// 6. Отправка письма
	if err := client.Mail(config.SMTP.From); err != nil {
		log.Printf("Error MAIL: %v", err)
		return
	}
	if err := client.Rcpt(config.SMTP.To); err != nil {
		log.Printf("Error RCPT: %v", err)
		return
	}

	wc, err := client.Data()
	if err != nil {
		log.Printf("Error DATA: %v", err)
		return
	}
	defer wc.Close()

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		config.SMTP.From,
		config.SMTP.To,
		subject,
		body,
	)

	if _, err = wc.Write([]byte(msg)); err != nil {
		log.Printf("Write Error: %v", err)
		return
	}

	log.Println("Email successfully sent")
}

type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		}
	}
	return nil, nil
}
