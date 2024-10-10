
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
Here's an example client in Go that connects to the server:

```
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
)

func main() {
	// Use the flag package to parse command-line arguments
	serverAddr := flag.String("server", "localhost:8080", "Address of the server to connect to")
	flag.Parse()

	// Connect to the server
	conn, err := net.Dial("tcp", *serverAddr)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Get username and password input from the user
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter Username: ")
	username, _ := reader.ReadString('\n')

	fmt.Print("Enter Password: ")
	password, _ := reader.ReadString('\n')

	// Send the username and password to the server
	conn.Write([]byte(username))
	conn.Write([]byte(password))

	// Continuously read from the server and display updates
	serverReader := bufio.NewReader(conn)
	for {
		message, err := serverReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from server:", err)
			break
		}
		fmt.Print(message)
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



