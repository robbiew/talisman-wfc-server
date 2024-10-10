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

	"github.com/hpcloud/tail"
	_ "modernc.org/sqlite"
)

// NodeStatus holds the current status of a node
type NodeStatus struct {
	User     string
	Location string
}

// Track the node statuses and update them
var nodeStatus = make(map[string]NodeStatus)

// Function to hash the password + salt using SHA-256
func hashPassword(password, salt string) string {
	concatenated := password + salt
	hash := sha256.Sum256([]byte(concatenated))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

// Authenticate user using the database and required seclevel
func authenticateUser(db *sql.DB, username, password string, requiredSeclevel int) (bool, error) {
	username = strings.TrimSpace(username)
	usernameLower := strings.ToLower(username)

	var dbPassword, salt, userID string

	query := `SELECT id, password, salt FROM users WHERE LOWER(username) = ?`
	err := db.QueryRow(query, usernameLower).Scan(&userID, &dbPassword, &salt)
	if err != nil {
		return false, fmt.Errorf("user not found: %v", err)
	}

	// Hash the provided password with the salt
	hashedPassword := hashPassword(password, salt)
	if hashedPassword != dbPassword {
		return false, fmt.Errorf("invalid password")
	}

	// Check the user's security level (seclevel)
	var seclevel int
	query = `SELECT value FROM details WHERE uid = ? AND attrib = 'seclevel'`
	err = db.QueryRow(query, userID).Scan(&seclevel)
	if err != nil {
		return false, fmt.Errorf("failed to fetch seclevel: %v", err)
	}

	// Ensure the seclevel meets the required minimum
	if seclevel < requiredSeclevel {
		return false, fmt.Errorf("insufficient seclevel: %d, required: %d", seclevel, requiredSeclevel)
	}

	// Authentication successful
	return true, nil
}

// Handle incoming client connections, including authentication and streaming logs
func handleClient(conn net.Conn, db *sql.DB, logFilePath string, requiredSeclevel int) {
	defer conn.Close()

	clientReader := bufio.NewReader(conn)
	clientWriter := bufio.NewWriter(conn)

	// Send the username prompt and flush
	clientWriter.WriteString("Username: \n")
	clientWriter.Flush()

	// Read the username from the client
	username, err := clientReader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading username from client:", err)
		return
	}
	username = strings.TrimSpace(username)

	// Send the password prompt and flush
	clientWriter.WriteString("Password: \n")
	clientWriter.Flush()

	// Read the password from the client
	password, err := clientReader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading password from client:", err)
		return
	}
	password = strings.TrimSpace(password)

	// Authenticate the user with the required seclevel
	authenticated, err := authenticateUser(db, username, password, requiredSeclevel)
	if err != nil {
		clientWriter.WriteString(fmt.Sprintf("Authentication failed: %v\n", err))
		clientWriter.Flush()
		fmt.Println("Authentication failed")
		return
	}

	if !authenticated {
		clientWriter.WriteString("Authentication failed: Invalid credentials or insufficient seclevel.\n")
		clientWriter.Flush()
		fmt.Println("User authentication failed")
		return
	}

	// Notify client of successful authentication
	clientWriter.WriteString("Authentication successful!\n")
	clientWriter.Flush()
	fmt.Println("User authenticated successfully")

	// Start streaming the log file to the client
	fmt.Println("Starting to stream log file:", logFilePath)
	t, err := tail.TailFile(logFilePath, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		fmt.Printf("Error tailing log file: %v\n", err)
		clientWriter.WriteString("Error: Could not stream log file\n")
		clientWriter.Flush()
		return
	}

	// Stream the log updates to the client in real time
	for line := range t.Lines {
		_, err := clientWriter.WriteString(line.Text + "\n")
		if err != nil {
			fmt.Println("Error writing to client:", err)
			return
		}
		clientWriter.Flush()
	}
}

func main() {
	// Parse command-line flags for the server port and required seclevel
	port := flag.String("port", "", "Port number for the server (required)")
	requiredSeclevel := flag.Int("seclevel", 100, "Required security level for user access (required)")
	pathPtr := flag.String("path", "", "Path to the BBS directory containing talisman.ini (required)")
	flag.Parse()

	// Ensure all required flags are provided
	if *port == "" || *requiredSeclevel == 100 || *pathPtr == "" {
		fmt.Println("Error: --port, --seclevel, and --path flags are required")
		flag.Usage()
		os.Exit(1)
	}

	// Load the configuration
	dataPath, logPath, err := getPathsFromIni(filepath.Join(*pathPtr, "talisman.ini"))
	if err != nil {
		fmt.Println("Error reading talisman.ini:", err)
		os.Exit(1)
	}

	// If the dataPath from the ini file is relative, combine it with the --path directory
	if !filepath.IsAbs(dataPath) {
		dataPath = filepath.Join(*pathPtr, dataPath)
	}

	// If the logPath from the ini file is relative, combine it with the --path directory
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(*pathPtr, logPath)
	}

	// Construct the full log file path
	logFilePath := filepath.Join(logPath, "talisman.log")

	// Connect to SQLite database
	db, err := connectToDatabase(dataPath)
	if err != nil {
		fmt.Println("Error connecting to database:", err)
		os.Exit(1)
	}
	defer db.Close()

	// Listen for incoming client connections
	listener, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("Server is running on port %s with required seclevel %d...\n", *port, *requiredSeclevel)

	// Accept and handle incoming connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		// Handle the client connection in a new goroutine, including log streaming
		go handleClient(conn, db, logFilePath, *requiredSeclevel)
	}
}

// Utility to connect to the SQLite database
func connectToDatabase(dataPath string) (*sql.DB, error) {
	dbPath := filepath.Join(dataPath, "users.sqlite3")

	// Debugging: Print the absolute path of the database
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

// Utility to read both "data path" and "log path" from talisman.ini
func getPathsFromIni(filename string) (string, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	var dataPath, logPath string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Look for "data path" in the ini file
		if strings.HasPrefix(line, "data path") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				dataPath = strings.TrimSpace(parts[1])
			}
		}

		// Look for "log path" in the ini file
		if strings.HasPrefix(line, "log path") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				logPath = strings.TrimSpace(parts[1])
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	if dataPath == "" {
		return "", "", fmt.Errorf("data path not found in talisman.ini file")
	}

	if logPath == "" {
		return "", "", fmt.Errorf("log path not found in talisman.ini file")
	}

	return dataPath, logPath, nil
}
