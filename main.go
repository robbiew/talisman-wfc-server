package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Function to read the ini file and return the data path
func getDataPathFromIni(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Look for "data path" in the ini file
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

// Connect to SQLite database using the extracted data path
func connectToDatabase(dataPath string) (*sql.DB, error) {
	// Assume that the users.sqlite3 file is in the data path
	dbPath := filepath.Join(dataPath, "users.sqlite3")

	// Debug: Print the absolute path
	absolutePath, _ := filepath.Abs(dbPath)
	fmt.Println("Trying to open database at absolute path:", absolutePath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Test if the database can be opened by pinging it
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %v", err)
	}

	return db, nil
}

func authenticateUser(db *sql.DB, username, password string) (bool, error) {
	// Trim any leading/trailing whitespace and make username case-insensitive
	username = strings.TrimSpace(username)
	usernameLower := strings.ToLower(username)

	var dbPassword, salt, userID string

	// Query the users table for the given username
	query := `SELECT id, password, salt FROM users WHERE LOWER(username) = ?`
	err := db.QueryRow(query, usernameLower).Scan(&userID, &dbPassword, &salt)
	if err != nil {
		return false, fmt.Errorf("user not found: %v", err)
	}

	// Try both password + salt and salt + password
	hashedPassword1 := sha256.Sum256([]byte(password + salt)) // Password + Salt
	hashedPasswordStr1 := fmt.Sprintf("%x", hashedPassword1)

	hashedPassword2 := sha256.Sum256([]byte(salt + password)) // Salt + Password
	hashedPasswordStr2 := fmt.Sprintf("%x", hashedPassword2)

	// Check if either hashed password matches the one in the database
	if hashedPasswordStr1 == dbPassword || hashedPasswordStr2 == dbPassword {
		// Check the user's seclevel
		var seclevel int
		query = `SELECT value FROM details WHERE uid = ? AND attrib = 'seclevel'`
		err = db.QueryRow(query, userID).Scan(&seclevel)
		if err != nil {
			return false, fmt.Errorf("failed to fetch seclevel: %v", err)
		}

		// Ensure seclevel is at least 100
		if seclevel < 100 {
			return false, fmt.Errorf("insufficient seclevel: %d", seclevel)
		}

		// Authentication successful
		return true, nil
	}

	// If neither hashed password matches, return an invalid password error
	return false, fmt.Errorf("invalid password")
}

// Handle an incoming client connection
func handleConnection(conn net.Conn, db *sql.DB) {
	defer conn.Close()

	// Set up reading from the client
	clientReader := bufio.NewReader(conn)

	// Ask for the username and password
	conn.Write([]byte("Username: "))
	username, _ := clientReader.ReadString('\n')
	username = strings.TrimSpace(username)

	conn.Write([]byte("Password: "))
	password, _ := clientReader.ReadString('\n')
	password = strings.TrimSpace(password)

	// Attempt to authenticate the user
	authenticated, err := authenticateUser(db, username, password)
	if err != nil {
		// Provide more detailed error messages for debugging purposes
		if strings.Contains(err.Error(), "user not found") {
			conn.Write([]byte("Authentication failed: user not found\n"))
		} else if strings.Contains(err.Error(), "invalid password") {
			conn.Write([]byte("Authentication failed: invalid password\n"))
		} else if strings.Contains(err.Error(), "insufficient seclevel") {
			conn.Write([]byte("Authentication failed: insufficient seclevel\n"))
		} else {
			conn.Write([]byte(fmt.Sprintf("Authentication failed: %v\n", err)))
		}
		return
	}

	// Notify the client of successful authentication
	if authenticated {
		conn.Write([]byte("Authentication successful!\n"))
	} else {
		conn.Write([]byte("Authentication failed: invalid credentials or insufficient seclevel.\n"))
	}
}

func main() {
	// Step 1: Use the flag package to parse the command line arguments
	pathPtr := flag.String("path", ".", "Path to the BBS directory containing talisman.ini")
	flag.Parse()

	// Step 2: Read the talisman.ini file to get the data path
	iniFile := filepath.Join(*pathPtr, "talisman.ini")
	dataPath, err := getDataPathFromIni(iniFile)
	if err != nil {
		fmt.Println("Error reading talisman.ini:", err)
		os.Exit(1)
	}

	// If the dataPath from the ini file is relative, combine it with the --path directory
	if !filepath.IsAbs(dataPath) {
		dataPath = filepath.Join(*pathPtr, dataPath)
	}

	fmt.Println("Final Data path:", dataPath)

	// Step 3: Load the users.sqlite3 database
	db, err := connectToDatabase(dataPath)
	if err != nil {
		fmt.Println("Error connecting to database:", err)
		os.Exit(1)
	}
	defer db.Close()

	// Step 4: Start the server and listen for incoming connections
	listener, err := net.Listen("tcp", ":8080") // Listen on port 8080, adjust as needed
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Server is running and waiting for connections on port 8080...")

	// Accept incoming connections and handle them
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		// Handle the connection in a separate goroutine
		go handleConnection(conn, db)
	}
}
