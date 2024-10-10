
# Talisman WFC Server

## Overview
The Talisman WFC (Waiting For Caller) server is designed to stream log information, BBS stats, and other details to a remote client. Currently, it streams the Talisman log. The application has been tested on Windows 10 and Ubuntu 24.04.

## Building the Application
To build the application, you need to have Go installed on your system. Run the following command to compile the application:

```bash
go build -o talisman-wfc-server main.go
```

## Using the Application
To run the Talisman WFC server, use the following command with the necessary flags:

```bash
./talisman-wfc-server --port <port-number> --seclevel <security-level> --path <path-to-bbs-directory>
```

### Required Flags
- `--port`: Port number for the server (required)
- `--seclevel`: Required security level for user access (required)
- `--path`: Path to the BBS directory containing `talisman.ini` (required)

### Example
```bash
./talisman-wfc-server --port 8080 --seclevel 100 --path /path/to/bbs
```

## Connecting with a Client Application
To connect to the Talisman WFC server with a client application, follow these steps:

1. Open a TCP connection to the server using the specified port.
2. Enter the username when prompted.
3. Enter the password when prompted.
4. Upon successful authentication, the server will start streaming the log file to the client.

## Client Code Example
Here's an example client in Go that connects to the server ([GitHub link](https://github.com/robbiew/talisman-wfc-server)):

```
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	// Define command-line flags
	serverAddr := flag.String("server", "", "Address of the server to connect to (e.g., localhost:8080)")

	// Parse command-line flags
	flag.Parse()

	// Ensure that the required flags are set
	if *serverAddr == "" {
		fmt.Println("Error: --server flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Connect to the server
	conn, err := net.Dial("tcp", *serverAddr)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Create readers for the server and user input
	serverReader := bufio.NewReader(conn)
	clientReader := bufio.NewReader(os.Stdin)

	// Step 1: Handle server's username and password prompts
	for {
		// Read the server's message (username or password prompt)
		prompt, err := serverReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from server:", err)
			return
		}
		fmt.Print(prompt)

		// If the server sends "Authentication successful", break out of this loop and continue
		if strings.Contains(prompt, "Authentication successful") {
			break
		}

		// Get user input for the username or password
		userInput, _ := clientReader.ReadString('\n')
		userInput = strings.TrimSpace(userInput)

		// Send the user input to the server
		conn.Write([]byte(userInput + "\n"))
	}

	// Step 2: Continuously read updates from the server after authentication
	for {
		update, err := serverReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading updates from server:", err)
			break
		}
		fmt.Print(update)
	}
}


```

Compile the client:
```
go build -o client client.go
```

Run the client and connect to the server:
```
./client --server localhost:8080
```


## Additional Information
The application authenticates users against an SQLite database and requires a certain security level for access. The log streaming is handled using the `tail` package to follow updates in real-time.



