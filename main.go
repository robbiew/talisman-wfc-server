package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func getDataPathFromIni(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data path") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("data path not found in ini file")
}

func connectToDatabase(dataPath string) (*sql.DB, error) {
	dbPath := filepath.Join(dataPath, "users.sqlite3")

	absolutePath, _ := filepath.Abs(dbPath)
	fmt.Println("Trying to open database at absolute path:", absolutePath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %v", err)
	}

	return db, nil
}

func hashPassword(password, salt string) string {
	concatenated := password + salt
	hash := sha256.Sum256([]byte(concatenated))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

func authenticateUser(db *sql.DB, username, password string) (bool, error) {
	username = strings.TrimSpace(username)
	usernameLower := strings.ToLower(username)

	var dbPassword, salt, userID string

	query := `SELECT id, password, salt FROM users WHERE LOWER(username) = ?`
	err := db.QueryRow(query, usernameLower).Scan(&userID, &dbPassword, &salt)
	if err != nil {
		return false, fmt.Errorf("user not found: %v", err)
	}

	hashedPassword := hashPassword(password, salt)
	if hashedPassword != dbPassword {
		return false, fmt.Errorf("invalid password")
	}

	var seclevel int
	query = `SELECT value FROM details WHERE uid = ? AND attrib = 'seclevel'`
	err = db.QueryRow(query, userID).Scan(&seclevel)
	if err != nil {
		return false, fmt.Errorf("failed to fetch seclevel: %v", err)
	}

	if seclevel < 100 {
		return false, fmt.Errorf("insufficient seclevel: %d", seclevel)
	}

	return true, nil
}

func handleConnection(conn net.Conn, db *sql.DB) {
	defer conn.Close()

	clientReader := bufio.NewReader(conn)
	clientWriter := bufio.NewWriter(conn)

	fmt.Println("New client connected")

	// Send the username prompt with newline and flush
	_, err := clientWriter.WriteString("Username: \n")
	if err != nil {
		fmt.Println("Error sending username prompt to client:", err)
		return
	}
	err = clientWriter.Flush()
	if err != nil {
		fmt.Println("Error flushing username prompt:", err)
		return
	}
	fmt.Println("Sent username prompt and flushed buffer")

	// Read the username from the client
	username, err := clientReader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading username from client:", err)
		return
	}
	username = strings.TrimSpace(username)
	fmt.Printf("Received username: %s\n", username)

	// Send the password prompt with newline and flush
	_, err = clientWriter.WriteString("Password: \n")
	if err != nil {
		fmt.Println("Error sending password prompt to client:", err)
		return
	}
	err = clientWriter.Flush()
	if err != nil {
		fmt.Println("Error flushing password prompt:", err)
		return
	}
	fmt.Println("Sent password prompt and flushed buffer")

	// Read the password from the client
	password, err := clientReader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading password from client:", err)
		return
	}
	password = strings.TrimSpace(password)
	fmt.Printf("Received password for user %s\n", username)

	// Attempt to authenticate the user
	authenticated, err := authenticateUser(db, username, password)
	if err != nil {
		clientWriter.WriteString(fmt.Sprintf("Authentication failed: %v\n", err))
		clientWriter.Flush()
		fmt.Println("Authentication failed")
		return
	}

	// Notify the client of successful authentication and flush
	if authenticated {
		clientWriter.WriteString("Authentication successful!\n")
		fmt.Println("User authenticated successfully")
	} else {
		clientWriter.WriteString("Authentication failed: invalid credentials or insufficient seclevel.\n")
		fmt.Println("User authentication failed")
	}
	clientWriter.Flush()
}

func main() {
	pathPtr := flag.String("path", ".", "Path to the BBS directory containing talisman.ini")
	flag.Parse()

	iniFile := filepath.Join(*pathPtr, "talisman.ini")
	dataPath, err := getDataPathFromIni(iniFile)
	if err != nil {
		fmt.Println("Error reading talisman.ini:", err)
		os.Exit(1)
	}

	if !filepath.IsAbs(dataPath) {
		dataPath = filepath.Join(*pathPtr, dataPath)
	}

	fmt.Println("Final Data path:", dataPath)

	db, err := connectToDatabase(dataPath)
	if err != nil {
		fmt.Println("Error connecting to database:", err)
		os.Exit(1)
	}
	defer db.Close()

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Server is running and waiting for connections on port 8080...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleConnection(conn, db)
	}
}
