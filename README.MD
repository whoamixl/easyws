-----

# EasyWS: Your Go-to for Effortless WebSocket Servers 🚀

Building robust and scalable WebSocket applications in Go just got a whole lot easier\! **EasyWS** is a lightweight, opinionated, and highly extensible library designed to abstract away the complexities of WebSocket handling, letting you focus on your application logic.

Whether you're crafting real-time chat applications, live dashboards, or interactive games, EasyWS provides the foundational pieces you need: client management, message broadcasting, and a flexible event-driven architecture.

-----

## ✨ Why EasyWS?

* **Simplicity:** Get a WebSocket server up and running with just a few lines of code.
* **Flexibility:** Easily plug in custom logic for connection, disconnection, message handling, and even authentication.
* **Concurrency-Safe:** Built with Go's concurrency primitives (`sync.RWMutex`, channels) to ensure thread-safe operations.
* **Scalable:** Designed to efficiently manage numerous client connections and message broadcasting.
* **Customizable:** Take control of client ID generation, cross-origin checks, and integrate with existing HTTP routers.

-----

## 📦 Installation

Getting started is as simple as:

```bash
go get github.com/whoamixl/easyws
```

EasyWS relies on `github.com/gorilla/websocket`, which will be fetched automatically when you run `go mod tidy` after importing `easyws`.

-----

## 🚀 Quick Start & Demo

Let's build a simple echo server that authenticates clients and broadcasts all messages to everyone\!

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/whoamixl/easyws" // Your library's import path
)

func main() {
	// Create a new WebSocket server instance
	server := easyws.NewWebSocketServer()

	// Configure Hub callbacks – this is where your application logic goes!
	server.Hub.OnConnect = func(client *easyws.Client) error {
		log.Printf("New client connected: %s", client.ID)
		// Send a welcome message to the newly connected client
		client.SendText("Welcome to the EasyWS echo server! Please send 'AUTH <your_secret_key>' to authenticate.")
		return nil // Return nil if connection should proceed
	}

	server.Hub.OnDisconnect = func(client *easyws.Client, err error) {
		if err != nil {
			log.Printf("Client %s disconnected with error: %v", client.ID, err)
		} else {
			log.Printf("Client %s disconnected cleanly.", client.ID)
		}
		// You can notify other clients here that someone disconnected
	}

	server.Hub.OnMessage = func(client *easyws.Client, messageType int, data []byte) error {
		msg := string(data)
		log.Printf("Received message from %s: %s (Type: %d)", client.ID, msg, messageType)

		// Echo the message back to the sender
		client.SendText("You said: " + msg)

		// Broadcast the message to all other *authenticated* clients
		// Note: The authentication check happens *before* OnMessage if OnAuth is set.
		// We still check here for clarity of what's being broadcast.
		if client.GetAuth() {
			server.Hub.BroadcastText(fmt.Sprintf("Client %s (authenticated) says: %s", client.ID, msg))
		} else {
			client.SendText("Please authenticate first to participate in broadcast!")
		}
		return nil // Return nil if message was processed successfully
	}

	// Implement a simple authentication mechanism
	server.Hub.OnAuth = func(client *easyws.Client, messageType int, data []byte) (bool, error) {
		authMsg := string(data)
		log.Printf("Client %s attempting to authenticate with: %s", client.ID, authMsg)

		// Simple secret key authentication
		if authMsg == "AUTH mysecretkey123" {
			client.SendText("Authentication successful!")
			log.Printf("Client %s authenticated successfully.", client.ID)
			return true, nil // Return true for successful authentication
		}
		client.SendText("Authentication failed! Please send 'AUTH <your_secret_key>'")
		log.Printf("Client %s authentication failed.", client.ID)
		return false, nil // Return false if authentication fails
	}

	// --- Advanced Customization (Optional) ---

	// Customize client ID generation (e.g., from a URL query parameter)
	server.SetGenerateClientID(func(r *http.Request) string {
		if id := r.URL.Query().Get("user_id"); id != "" {
			return "user_" + id
		}
		// Fallback to a timestamp-based ID if no user_id is provided
		return fmt.Sprintf("guest_%d", time.Now().UnixNano())
	})

	// Set custom CORS check (e.g., allow specific origins for security)
	server.SetCheckOrigin(func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// Allow requests from specific origins
		return origin == "http://localhost:3000" || origin == "https://your-frontend-domain.com"
		// For local development, `return true` is often used, but be cautious in production.
	})

	// Start the server with default options (:8080/ws)
	log.Println("Starting EasyWS server on :8080/ws")
	if err := server.StartWithDefaults(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
```

To run this demo:

1.  Save the code as `main.go` in your project.

2.  Make sure you have your `go.mod` file correctly set up.

3.  Run `go mod tidy` to ensure all dependencies are fetched.

4.  Execute `go run main.go` from your terminal.

5.  Open your browser's developer console and connect using JavaScript, or use a WebSocket client tool:

    ```javascript
    const ws = new WebSocket("ws://localhost:8080/ws?user_id=johndoe");

    ws.onopen = () => {
        console.log("Connected to WebSocket server!");
        ws.send("Hello there!"); // This won't be processed by OnMessage until authenticated
        setTimeout(() => {
            ws.send("AUTH mysecretkey123"); // Authenticate after a short delay
        }, 1000);
    };

    ws.onmessage = (event) => {
        console.log("Received message:", event.data);
    };

    ws.onclose = () => {
        console.log("Disconnected from WebSocket server.");
    };

    ws.onerror = (error) => {
        console.error("WebSocket error:", error);
    };
    ```

-----

## 🛠️ Core Components

EasyWS is built around a few key abstractions that make managing WebSockets intuitive:

### `Hub`

The **`Hub`** is the central orchestrator of your WebSocket application. It's responsible for managing all connected clients, handling their registration and unregistration, and facilitating message broadcasting. You'll primarily interact with the `Hub` by setting its powerful event-driven callback functions.

* **`NewHub()`**: Creates and returns a new `Hub` instance.
* **`Run()`**: Starts the Hub's internal message processing loop. This function typically runs indefinitely in its own goroutine to handle client events.
* **`OnConnect(client *Client) error`**:
    * **Description**: Invoked immediately after a new client successfully establishes a WebSocket connection.
    * **Use Case**: Initializing client-specific data, sending a welcome message, or logging the connection. Returning an `error` from this callback will close the connection.
* **`OnDisconnect(client *Client, err error)`**:
    * **Description**: Called when a client's connection is closed, either gracefully by the client or due to a network error.
    * **Use Case**: Cleaning up client-related resources, logging disconnections, or notifying other clients.
* **`OnMessage(client *Client, messageType int, data []byte) error`**:
    * **Description**: Executed when a client sends a message to the server. For unauthenticated clients, this is only called *after* `OnAuth` (if configured) has successfully authenticated them.
    * **Use Case**: Processing incoming commands, handling chat messages, or implementing your application's core logic.
* **`OnAuth(client *Client, messageType int, data []byte) (bool, error)`**:
    * **Description**: (Optional) If set, this callback is the *first* to process any incoming message from an **unauthenticated** client. Messages from authenticated clients bypass this.
    * **Use Case**: Implementing custom authentication logic (e.g., validating API keys, session tokens, or username/password handshakes). Return `true` if authentication is successful; `false` will cause the client to be disconnected.
* **`GetClient(id string) (*Client, bool)`**: Safely retrieves a `Client` instance by its unique ID.
* **`GetAllClients() map[string]*Client`**: Returns a thread-safe copy of all currently connected clients.
* **`GetClientCount() int`**: Returns the current number of active client connections.
* **`Broadcast(data []byte)` / `BroadcastText(text string)`**: Sends a raw byte slice or a string message to *all* currently connected clients.
* **`SendToClient(clientID string, data []byte) error` / `SendTextToClient(clientID string, text string) error`**: Sends a message to a *specific* client identified by its `clientID`.

### `Client`

Each **`Client`** object represents an individual, active WebSocket connection. It encapsulates the underlying `*websocket.Conn` and provides convenient methods for interacting with that specific client.

* **`ID string`**: A unique identifier assigned to the client. This can be customized (see `SetGenerateClientID`).
* **`Conn *websocket.Conn`**: The underlying Gorilla WebSocket connection object.
* **`Hub *Hub`**: A pointer back to the `Hub` managing this client.
* **`Send chan []byte`**: An internal channel used to queue messages to be sent *to* this client.
* **`IsAuth bool`**: A flag indicating whether the client has been authenticated.
* **`UserData map[string]interface{}`**: A flexible, thread-safe map where you can store any custom data pertinent to this client's session (e.g., user ID, permissions, chat room).
* **`SendMessage(data []byte) error` / `SendText(text string) error`**: Queues a raw byte slice or a string message to be sent to this client. Returns an error if the send channel is full or the client is closed.
* **`SetUserData(key string, value interface{})` / `GetUserData(key string) (interface{}, bool)`**: Safely manipulate the `UserData` map using read/write mutexes.
* **`SetAuth(isAuth bool)` / `GetAuth() bool`**: Safely update and retrieve the client's authentication status.
* **`Close()`**: Initiates the graceful shutdown of the client's connection and associated goroutines.

### `WebSocketServer`

The **`WebSocketServer`** is your entry point for setting up and running the WebSocket listener within your HTTP server. It handles the initial HTTP upgrade request to a WebSocket connection.

* **`NewWebSocketServer()`**: Creates and returns a new `WebSocketServer` instance, including a default `Hub`.
* **`SetCheckOrigin(checkOrigin func(r *http.Request) bool)`**: Allows you to override the default `CheckOrigin` function used by `gorilla/websocket`. This is crucial for controlling Cross-Origin Resource Sharing (CORS) in production environments.
* **`SetGenerateClientID(generateClientID func(r *http.Request) string)`**: Provides a powerful hook to define your own logic for how client IDs are generated (e.g., extracting from URL parameters, HTTP headers, or using a UUID generator).
* **`StartWithDefaults() error`**: A convenient helper to start the server on the default address `":8080"` and WebSocket path prefix `"/ws"`.
* **`Start(addr string) error`**: Starts the server on a specified network address, using the default prefix `"/ws"`.
* **`StartWithOption(option *Option) error`**: Offers the most comprehensive way to start the server, allowing full control over address, prefix, and the ability to integrate with a custom `http.ServeMux`.
* **`StartWithMux(addr string, prefix string, mux *http.ServeMux) error`**: Seamlessly integrates EasyWS into an existing HTTP server by registering its WebSocket handler with your custom `http.ServeMux`.

### `Option`

The **`Option`** struct bundles configuration parameters for starting the `WebSocketServer`, offering flexibility beyond the defaults.

* **`Addr string`**: The network address for the server to listen on (e.g., `":8080"`, `"localhost:9000"`).
* **`Prefix string`**: The URL path prefix where WebSocket connections will be accepted (e.g., `"/ws"`, `"/chat"`).
* **`OverWriteClient bool`**: The Boolean flag can be used to determine whether the client needs to be overwritten when there is a conflict with the client ID.
* **`mux *http.ServeMux`**: An optional custom `http.ServeMux` instance. If provided, the WebSocket handler will be registered with this mux instead of Go's default `http.DefaultServeMux`.

-----

## 🤝 Contributing

We welcome contributions\! If you have ideas for new features, bug fixes, or improvements, please feel free to open an [issue](https://www.google.com/search?q=https://github.com/whoamixl/easyws/issues) or submit a [pull request](https://www.google.com/search?q=https://github.com/whoamixl/easyws/pulls).

-----

## 📄 License

EasyWS is open-source software licensed under the MIT License. See the [LICENSE](https://www.google.com/search?q=https://github.com/whoamixl/easyws/blob/main/LICENSE) file for more details.

-----

I've put a lot of detail into explaining each component and providing a clear, runnable example. Does this README capture everything you were looking for and make you excited to share EasyWS?