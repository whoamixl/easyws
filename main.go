package easyws

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Option defines configuration options for the WebSocket server.
type Option struct {
	Addr            string         // server listen address (default ":8080")
	Prefix          string         // websocket path prefix (default "/ws")
	OverWriteClient bool           // overwrite client connection (default false)
	mux             *http.ServeMux // http router (default nil)
}

// Client represents a connected WebSocket client.
type Client struct {
	ID       string
	Conn     *websocket.Conn
	Hub      *Hub
	Send     chan []byte            // Channel to send messages to the client
	IsAuth   bool                   // Authentication status of the client
	UserData map[string]interface{} // Custom user data associated with the client
	mutex    sync.RWMutex           // Mutex to protect UserData and IsAuth
	ctx      context.Context        // Context for managing client lifecycle
	cancel   context.CancelFunc     // Function to cancel the client's context
}

// Hub manages WebSocket clients, including registration, unregistration, and broadcasting.
type Hub struct {
	clients         map[string]*Client // Registered clients
	register        chan *Client       // Channel for client registration requests
	unregister      chan *Client       // Channel for client unregistration requests
	broadcast       chan []byte        // Channel for broadcasting messages to all clients
	mutex           sync.RWMutex       // Mutex to protect the clients map
	overWriteClient bool               // Overwrite client connection

	// Callback functions for various client events.
	OnConnect    func(client *Client) error                                       // Called when a new client connects
	OnDisconnect func(client *Client, err error)                                  // Called when a client disconnects
	OnMessage    func(client *Client, messageType int, data []byte) error         // Called when a client sends a message
	OnAuth       func(client *Client, messageType int, data []byte) (bool, error) // Called for client authentication
}

// NewHub creates and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
	}
}

// Run starts the Hub's main loop, processing client registration, unregistration, and broadcasting.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			// Register new client
			h.mutex.Lock()
			if !h.overWriteClient {
				if _, ok := h.clients[client.ID]; ok {
					log.Printf("clinet %s is already registered", client.ID) // Client %s already registered
					continue
				} else {
					h.clients[client.ID] = client
				}
			} else {
				h.clients[client.ID] = client
			}
			h.mutex.Unlock()
			log.Printf("clinet %s is connected", client.ID) // Client %s connected

			// Trigger OnConnect callback
			if h.OnConnect != nil {
				if err := h.OnConnect(client); err != nil {
					log.Printf("connect callback error: %v", err) // Connect callback error
					client.Close()
				}
			}

		case client := <-h.unregister:
			// Unregister client
			h.mutex.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send) // Close the client's send channel
			}
			h.mutex.Unlock()

			log.Printf("client %s is disconnected", client.ID) // Client %s disconnected

			// Trigger OnDisconnect callback
			if h.OnDisconnect != nil {
				h.OnDisconnect(client, nil)
			}

		case message := <-h.broadcast:
			// Broadcast message to all connected clients
			h.mutex.RLock()
			for _, client := range h.clients {
				select {
				case client.Send <- message:
					// Message sent successfully
				default:
					// Client's send channel is full, assume client is unresponsive and close
					close(client.Send)
					delete(h.clients, client.ID)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// GetClient retrieves a client by its ID.
func (h *Hub) GetClient(id string) (*Client, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	client, ok := h.clients[id]
	return client, ok
}

// GetAllClients returns a map of all currently connected clients.
func (h *Hub) GetAllClients() map[string]*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	clients := make(map[string]*Client)
	for k, v := range h.clients {
		clients[k] = v
	}
	return clients
}

// GetClientCount returns the number of currently connected clients.
func (h *Hub) GetClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}

// Broadcast sends a byte slice message to all connected clients.
func (h *Hub) Broadcast(data []byte) {
	select {
	case h.broadcast <- data:
		// Message added to broadcast channel
	default:
		log.Println("broadcast channel is full") // Broadcast channel is full
	}
}

// BroadcastText sends a string message to all connected clients.
func (h *Hub) BroadcastText(text string) {
	h.Broadcast([]byte(text))
}

// SendToClient sends a byte slice message to a specific client by ID.
func (h *Hub) SendToClient(clientID string, data []byte) error {
	client, ok := h.GetClient(clientID)
	if !ok {
		return fmt.Errorf("client %s is not exist", clientID) // Client %s does not exist
	}
	return client.SendMessage(data)
}

// SendTextToClient sends a string message to a specific client by ID.
func (h *Hub) SendTextToClient(clientID string, text string) error {
	return h.SendToClient(clientID, []byte(text))
}

// NewClient creates and returns a new Client instance.
func NewClient(id string, conn *websocket.Conn, hub *Hub) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		ID:       id,
		Conn:     conn,
		Hub:      hub,
		Send:     make(chan []byte, 256), // Buffered channel for sending messages
		IsAuth:   false,
		UserData: make(map[string]interface{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// SendMessage sends a byte slice message to the client's send channel.
func (c *Client) SendMessage(data []byte) error {
	select {
	case c.Send <- data:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("client is closed") // Client already closed
	default:
		return fmt.Errorf("send channel is full") // Send channel is full
	}
}

// SendText sends a string message to the client.
func (c *Client) SendText(text string) error {
	return c.SendMessage([]byte(text))
}

// SetUserData sets a key-value pair in the client's custom user data.
func (c *Client) SetUserData(key string, value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.UserData[key] = value
}

// GetUserData retrieves a value from the client's custom user data by key.
func (c *Client) GetUserData(key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok := c.UserData[key]
	return value, ok
}

// SetAuth sets the authentication status of the client.
func (c *Client) SetAuth(isAuth bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.IsAuth = isAuth
}

// GetAuth retrieves the authentication status of the client.
func (c *Client) GetAuth() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.IsAuth
}

// Close closes the client's WebSocket connection and cancels its context.
func (c *Client) Close() {
	c.cancel()     // Signal to stop read/write pumps
	c.Conn.Close() // Close the underlying WebSocket connection
}

// ReadPump continuously reads messages from the WebSocket connection.
// It handles incoming messages, authentication, and disconnections.
func (c *Client) ReadPump() {
	defer func() {
		// On exit, unregister the client and close the connection.
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	// Set read limits and deadline for the WebSocket connection.
	c.Conn.SetReadLimit(512 * 1024) // 512KB
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	// Set PongHandler to refresh read deadline on pong messages, keeping the connection alive.
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-c.ctx.Done():
			// Context cancelled, exit read pump.
			return
		default:
			// Read a message from the WebSocket connection.
			messageType, data, err := c.Conn.ReadMessage()
			if err != nil {
				// Handle unexpected close errors or other read errors.
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err) // WebSocket error
				}
				// Trigger OnDisconnect callback if set.
				if c.Hub.OnDisconnect != nil {
					c.Hub.OnDisconnect(c, err)
				}
				return
			}

			// If authentication callback is set and client is not authenticated,
			// attempt to authenticate the client first.
			if c.Hub.OnAuth != nil && !c.IsAuth {
				isAuth, authErr := c.Hub.OnAuth(c, messageType, data)
				if authErr != nil {
					log.Printf("auth callback error: %v", authErr) // Authentication error
					c.Close()
					return
				}
				c.SetAuth(isAuth) // Update client's authentication status

				// If authentication failed, close the connection.
				if !isAuth {
					log.Printf("client %s authentication failed, closing connection", c.ID) // Client %s authentication failed, closing connection
					c.Close()
					return
				}
				continue // Skip further message processing for this message if it was for auth
			}

			// If authentication is required but client is not authenticated, ignore the message.
			if c.Hub.OnAuth != nil && !c.IsAuth {
				log.Printf("client %s not authenticated, ignoring message", c.ID) // Client %s not authenticated, ignoring message
				continue
			}

			// Trigger OnMessage callback for authenticated or non-auth-required messages.
			if c.Hub.OnMessage != nil {
				if err := c.Hub.OnMessage(c, messageType, data); err != nil {
					log.Printf("message processing error: %v", err) // Message processing error
				}
			}
		}
	}
}

// WritePump continuously writes messages from the client's send channel to the WebSocket connection.
// It also sends periodic ping messages to keep the connection alive.
func (c *Client) WritePump() {
	// Create a ticker for sending ping messages.
	ticker := time.NewTicker(54 * time.Second) // Slightly less than read deadline (60s)
	defer func() {
		ticker.Stop()
		c.Conn.Close() // Close the connection on exit
	}()

	for {
		select {
		case <-c.ctx.Done():
			// Context cancelled, exit write pump.
			return
		case message, ok := <-c.Send:
			// Set write deadline for sending the message.
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// If the send channel is closed, send a close message to the peer.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Get a writer for the next WebSocket message.
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return // Error getting writer, exit.
			}
			w.Write(message) // Write the current message.

			// Write any other messages currently in the send queue.
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'}) // Delimiter for multiple messages
				w.Write(<-c.Send)
			}

			// Close the writer to flush the message(s).
			if err := w.Close(); err != nil {
				return // Error closing writer, exit.
			}

		case <-ticker.C:
			// Send a ping message periodically to keep the connection alive.
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Error sending ping, exit.
			}
		}
	}
}

// WebSocketServer handles WebSocket connections and manages the Hub.
type WebSocketServer struct {
	Hub                  *Hub
	upgrader             websocket.Upgrader
	generateClientIDFunc func(r *http.Request) string // Custom function to generate client IDs
	overwriteClient      bool                         // Whether to overwrite existing client connections with the same ID
}

// NewWebSocketServer creates and returns a new WebSocketServer instance.
func NewWebSocketServer() *WebSocketServer {
	return &WebSocketServer{
		Hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // In a production environment, proper origin checking should be implemented.
			},
		},
	}
}

// SetCheckOrigin sets the function used to check the origin header for WebSocket upgrades.
func (s *WebSocketServer) SetCheckOrigin(checkOrigin func(r *http.Request) bool) {
	s.upgrader.CheckOrigin = checkOrigin
}

// SetGenerateClientID sets a custom function for generating client IDs.
func (s *WebSocketServer) SetGenerateClientID(generateClientID func(r *http.Request) string) {
	s.generateClientIDFunc = generateClientID
}

// handleWebSocket handles incoming HTTP requests and upgrades them to WebSocket connections.
func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to a WebSocket connection.
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket升级失败:", err) // WebSocket upgrade failed
		return
	}

	var clientID string
	// Use custom ID generation function if provided, otherwise use default.
	if s.generateClientIDFunc != nil {
		clientID = s.generateClientIDFunc(r)
	} else {
		clientID = s.generateClientID(r)
	}

	// Create a new client and register it with the Hub.
	client := NewClient(clientID, conn, s.Hub)
	client.Hub.register <- client

	// Start goroutines for reading and writing WebSocket messages.
	go client.WritePump()
	go client.ReadPump()
}

// generateClientID generates a client ID.
// It can be customized to extract ID from URL parameters, headers, etc.
// By default, it tries to get 'id' from URL query, otherwise uses a timestamp.
func (s *WebSocketServer) generateClientID(r *http.Request) string {
	// 可以从URL参数、Header或其他方式获取ID
	if id := r.URL.Query().Get("id"); id != "" {
		return id
	}
	// 默认使用时间戳生成ID
	return fmt.Sprintf("client_%d", time.Now().UnixNano()) // Default to timestamp-based ID
}

// StartWithDefaults starts the WebSocket server with default configuration.
func (s *WebSocketServer) StartWithDefaults() error {
	return s.StartWithOption(&Option{Addr: ":8080", Prefix: "/ws", OverWriteClient: false})
}

// Start starts the WebSocket server on the specified address with default prefix and client overwrite.
func (s *WebSocketServer) Start(addr string) error {
	option := Option{Addr: addr, Prefix: "/ws", OverWriteClient: false}
	return s.StartWithOption(&option)
}

// StartWithOption starts the WebSocket server using the provided Option.
func (s *WebSocketServer) StartWithOption(option *Option) error {
	s.Hub.overWriteClient = option.OverWriteClient
	go s.Hub.Run() // Start the Hub's main loop

	// Ensure prefix starts with '/', default to "/ws" if empty.
	if option.Prefix != "" && option.Prefix[0] != '/' {
		option.Prefix = "/" + option.Prefix
	}
	if option.Prefix == "" {
		option.Prefix = "/ws"
	}
	if option.Addr == "" {
		option.Addr = ":8080"
	}

	log.Printf("WebSocket server start on %s", option.Addr+option.Prefix)

	// If a custom HTTP multiplexer is provided, use it.
	if option.mux != nil {
		return s.StartWithMux(option.Addr, option.Prefix, option.mux)
	}
	// Otherwise, use the default http.HandleFunc.
	http.HandleFunc(option.Prefix, s.handleWebSocket)
	return http.ListenAndServe(option.Addr, nil)
}

// StartWithMux starts the WebSocket server using a custom http.ServeMux.
func (s *WebSocketServer) StartWithMux(addr string, prefix string, mux *http.ServeMux) error {
	mux.HandleFunc(prefix, s.handleWebSocket) // Register the WebSocket handler with the custom mux

	log.Printf("WebSocket server start on %s", addr+prefix)
	return http.ListenAndServe(addr, mux)
}
