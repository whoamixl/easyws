Here's a `README.md` for your `easyws` package, focusing on usage and callback explanations:

---

# easyws: A Simple and Easy-to-Use WebSocket Library for Go

`easyws` is a Go package that provides a straightforward and flexible way to build WebSocket servers. It handles common WebSocket patterns like client management, message broadcasting, and event-driven callbacks, allowing you to focus on your application logic.

## Features

* **Simple Client & Hub Architecture:** Easily manage connected WebSocket clients through a central Hub.
* **Message Broadcasting:** Broadcast messages to all connected clients.
* **Individual Client Messaging:** Send messages to specific clients by their ID.
* **Event-Driven Callbacks:** Define custom logic for connection, disconnection, message reception, and authentication events.
* **Built-in Authentication Flow:** Supports an optional authentication step for new connections.
* **Configurable WebSocket Upgrader:** Customize `CheckOrigin` and other `websocket.Upgrader` options.
* **Heartbeat (Ping/Pong):** Automatic ping/pong handling to keep connections alive.

## Installation

```bash
go get github.com/whoamixl/easyws 
```

## Usage

### 1. Basic Server Setup

Here's a minimal example to get an `easyws` server up and running:

```go
package main

import (
	"log"
	"net/http"

	"github.com/whoamixl/easyws" 
)

func main() {
	server := easyws.NewWebSocketServer()

	// Optional: Set custom callback functions
	server.Hub.OnConnect = func(client *easyws.Client) error {
		log.Printf("Client %s connected!", client.ID)
		client.SendText("Welcome to the easyws server!")
		return nil
	}

	server.Hub.OnDisconnect = func(client *easyws.Client, err error) {
		if err != nil {
			log.Printf("Client %s disconnected with error: %v", client.ID, err)
		} else {
			log.Printf("Client %s disconnected.", client.ID)
		}
	}

	server.Hub.OnMessage = func(client *easyws.Client, messageType int, data []byte) error {
		log.Printf("Received message from %s: %s", client.ID, string(data))
		// Echo the message back
		return client.SendText("You said: " + string(data))
	}

	// Start the server
	log.Fatal(server.Start(":8080"))
}
```

You can then connect to `ws://localhost:8080/ws` using a WebSocket client (e.g., a browser's JavaScript `WebSocket` API, or a tool like Postman/Insomnia).

### 2. Using Custom HTTP Mux

If you're already using a custom `http.ServeMux` for your application, you can integrate `easyws` with it:

```go
package main

import (
	"log"
	"net/http"

	"github.com/whoamixl/easyws" // 
)

func main() {
	server := easyws.NewWebSocketServer()
	mux := http.NewServeMux()

	// Your other HTTP routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from HTTP server!"))
	})

	// Register the WebSocket handler
	log.Fatal(server.StartWithMux(":8080", mux))
}
```

### 3. Authentication

The `easyws` package provides an `OnAuth` callback for handling client authentication. If `OnAuth` is set, a client's messages will only be processed by `OnMessage` *after* successful authentication.

```go
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/whoamixl/easyws"
)

type AuthMessage struct {
	Token string `json:"token"`
}

func main() {
	server := easyws.NewWebSocketServer()

	// Authentication Callback
	server.Hub.OnAuth = func(client *easyws.Client, messageType int, data []byte) (bool, error) {
		var authMsg AuthMessage
		if err := json.Unmarshal(data, &authMsg); err != nil {
			log.Printf("Auth error: Invalid JSON - %v", err)
			return false, nil // Authentication failed due to bad format
		}

		// Simulate a token validation
		if authMsg.Token == "mysecrettoken123" {
			client.SetUserData("username", "authenticated_user")
			client.SendText("Authentication successful!")
			log.Printf("Client %s authenticated.", client.ID)
			return true, nil // Authentication successful
		}

		client.SendText("Authentication failed: Invalid token.")
		log.Printf("Client %s authentication failed (invalid token).", client.ID)
		return false, nil // Authentication failed
	}

	// Message Callback (only called for authenticated clients if OnAuth is set)
	server.Hub.OnMessage = func(client *easyws.Client, messageType int, data []byte) error {
		username, _ := client.GetUserData("username")
		log.Printf("Authenticated client %s (%v) sent: %s", client.ID, username, string(data))
		return client.SendText(fmt.Sprintf("Hello %v, you said: %s", username, string(data)))
	}

	log.Fatal(server.Start(":8080"))
}
```

With the above `OnAuth` setup, a client would typically send an initial JSON message like `{"token": "mysecrettoken123"}` after connecting to authenticate.

### 4. Broadcasting Messages

You can broadcast messages to all connected clients or send messages to specific clients:

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/whoamixl/easyws"
)

func main() {
	server := easyws.NewWebSocketServer()

	server.Hub.OnConnect = func(client *easyws.Client) error {
		log.Printf("Client %s connected. Total clients: %d", client.ID, server.Hub.GetClientCount())
		server.Hub.BroadcastText(fmt.Sprintf("A new client %s has joined!", client.ID))
		return nil
	}

	server.Hub.OnMessage = func(client *easyws.Client, messageType int, data []byte) error {
		msg := string(data)
		log.Printf("Received from %s: %s", client.ID, msg)

		// Example: Send a private message to a specific client
		if msg == "send private" {
			targetID := "some_other_client_id" // You'd get this from your application logic
			if err := server.Hub.SendTextToClient(targetID, "This is a private message for you!"); err != nil {
				log.Printf("Error sending private message: %v", err)
			}
		} else {
			// Broadcast all other messages
			server.Hub.BroadcastText(fmt.Sprintf("%s says: %s", client.ID, msg))
		}
		return nil
	}

	go func() {
		// Example of broadcasting from outside the Hub's Run loop
		for {
			time.Sleep(5 * time.Second)
			server.Hub.BroadcastText(fmt.Sprintf("Server broadcast at %s", time.Now().Format(time.RFC3339)))
		}
	}()

	log.Fatal(server.Start(":8080"))
}
```

## Callbacks Explained

The `Hub` struct exposes several callback functions that you can set to define your application's behavior for different WebSocket events.

* `OnConnect func(client *Client) error`
    * **Triggered:** When a new WebSocket client successfully connects and is registered with the Hub.
    * **Parameters:**
        * `client *easyws.Client`: The newly connected client.
    * **Return:** An `error`. If you return a non-nil error, the client connection will be immediately closed. This is useful for denying connections based on initial checks (e.g., request headers, query parameters).

* `OnDisconnect func(client *Client, err error)`
    * **Triggered:** When a client disconnects from the WebSocket server. This can happen due to the client closing the connection, a network error, or the server explicitly closing the connection.
    * **Parameters:**
        * `client *easyws.Client`: The client that disconnected.
        * `err error`: The error that caused the disconnection, if any. This will be `nil` if the disconnection was graceful (e.g., client closed the connection normally).

* `OnMessage func(client *Client, messageType int, data []byte) error`
    * **Triggered:** When a message is received from a connected client.
    * **Parameters:**
        * `client *easyws.Client`: The client from whom the message was received.
        * `messageType int`: The WebSocket message type (e.g., `websocket.TextMessage`, `websocket.BinaryMessage`).
        * `data []byte`: The raw message data.
    * **Return:** An `error`. If you return a non-nil error, it will be logged, but the client connection will remain open unless a critical WebSocket error occurs.

* `OnAuth func(client *Client, messageType int, data []byte) (bool, error)`
    * **Triggered:** (Optional) If set, this callback is triggered for the *first message* received from a client that has not yet been authenticated (`client.IsAuth` is `false`).
    * **Parameters:** Same as `OnMessage`.
    * **Return:**
        * `bool`: `true` if authentication is successful, `false` otherwise.
        * `error`: An error if an issue occurred during the authentication *attempt* (e.g., parsing the authentication message).
    * **Behavior:**
        * If `OnAuth` returns `true` for authentication status, the client's `IsAuth` flag is set to `true`, and subsequent messages from this client will trigger `OnMessage`.
        * If `OnAuth` returns `false` for authentication status (or a non-nil error), the client's connection will be closed.
        * If `OnAuth` is set, and a client is not authenticated, any messages received from that client *before* successful authentication will only go through `OnAuth` and *not* `OnMessage`.

## Client Management

The `Hub` provides methods for managing and interacting with connected clients:

* `hub.GetClient(id string) (*Client, bool)`: Retrieves a client by its ID.
* `hub.GetAllClients() map[string]*Client`: Returns a copy of all connected clients.
* `hub.GetClientCount() int`: Returns the current number of connected clients.
* `hub.Broadcast(data []byte)`: Sends binary data to all connected clients.
* `hub.BroadcastText(text string)`: Sends a text string to all connected clients.
* `hub.SendToClient(clientID string, data []byte) error`: Sends binary data to a specific client.
* `hub.SendTextToClient(clientID string, text string) error`: Sends a text string to a specific client.

## Client Information and Actions

Each `Client` object provides methods to manage its state and send messages:

* `client.ID string`: The unique identifier for the client.
* `client.IsAuth bool`: Current authentication status of the client (managed by `SetAuth` and `OnAuth`).
* `client.UserData map[string]interface{}`: A thread-safe map to store custom data associated with the client.
    * `client.SetUserData(key string, value interface{})`: Sets a key-value pair in `UserData`.
    * `client.GetUserData(key string) (interface{}, bool)`: Retrieves a value from `UserData`.
* `client.SendMessage(data []byte) error`: Sends binary data to this specific client.
* `client.SendText(text string) error`: Sends a text string to this specific client.
* `client.SetAuth(isAuth bool)`: Manually sets the authentication status of the client.
* `client.GetAuth() bool`: Gets the current authentication status.
* `client.Close()`: Closes the client's WebSocket connection.

## Customizing WebSocket Upgrader

You can configure the `websocket.Upgrader` used by `easyws`, for example, to set your own `CheckOrigin` function for security:

```go
package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/whoamixl/easyws"
)

func main() {
	server := easyws.NewWebSocketServer()

	// Allow connections only from specific origins
	server.SetCheckOrigin(func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "https://your-domain.com")
	})

	log.Fatal(server.Start(":8080"))
}
```

---