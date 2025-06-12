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

// Client 客户端连接
type Client struct {
	ID       string
	Conn     *websocket.Conn
	Hub      *Hub
	Send     chan []byte
	IsAuth   bool
	UserData map[string]interface{}
	mutex    sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// Hub WebSocket集线器
type Hub struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mutex      sync.RWMutex

	// 回调函数
	OnConnect    func(client *Client) error
	OnDisconnect func(client *Client, err error)
	OnMessage    func(client *Client, messageType int, data []byte) error
	OnAuth       func(client *Client, messageType int, data []byte) (bool, error)
}

// NewHub 创建新的Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
	}
}

// Run 启动Hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client.ID] = client
			h.mutex.Unlock()

			log.Printf("客户端 %s 已连接", client.ID)

			// 触发连接回调
			if h.OnConnect != nil {
				if err := h.OnConnect(client); err != nil {
					log.Printf("连接回调错误: %v", err)
					client.Close()
				}
			}

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send)
			}
			h.mutex.Unlock()

			log.Printf("客户端 %s 已断开", client.ID)

			// 触发断开回调
			if h.OnDisconnect != nil {
				h.OnDisconnect(client, nil)
			}

		case message := <-h.broadcast:
			h.mutex.RLock()
			for _, client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client.ID)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// GetClient 获取客户端
func (h *Hub) GetClient(id string) (*Client, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	client, ok := h.clients[id]
	return client, ok
}

// GetAllClients 获取所有客户端
func (h *Hub) GetAllClients() map[string]*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	clients := make(map[string]*Client)
	for k, v := range h.clients {
		clients[k] = v
	}
	return clients
}

// GetClientCount 获取客户端数量
func (h *Hub) GetClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}

// Broadcast 广播消息到所有客户端
func (h *Hub) Broadcast(data []byte) {
	select {
	case h.broadcast <- data:
	default:
		log.Println("广播通道已满")
	}
}

// BroadcastText 广播文本消息到所有客户端
func (h *Hub) BroadcastText(text string) {
	h.Broadcast([]byte(text))
}

// SendToClient 发送消息给指定客户端
func (h *Hub) SendToClient(clientID string, data []byte) error {
	client, ok := h.GetClient(clientID)
	if !ok {
		return fmt.Errorf("客户端 %s 不存在", clientID)
	}
	return client.SendMessage(data)
}

// SendTextToClient 发送文本消息给指定客户端
func (h *Hub) SendTextToClient(clientID string, text string) error {
	return h.SendToClient(clientID, []byte(text))
}

// NewClient 创建新客户端
func NewClient(id string, conn *websocket.Conn, hub *Hub) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		ID:       id,
		Conn:     conn,
		Hub:      hub,
		Send:     make(chan []byte, 256),
		IsAuth:   false,
		UserData: make(map[string]interface{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// SendMessage 发送二进制消息
func (c *Client) SendMessage(data []byte) error {
	select {
	case c.Send <- data:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("客户端已关闭")
	default:
		return fmt.Errorf("发送通道已满")
	}
}

// SendText 发送文本消息
func (c *Client) SendText(text string) error {
	return c.SendMessage([]byte(text))
}

// SetUserData 设置用户数据
func (c *Client) SetUserData(key string, value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.UserData[key] = value
}

// GetUserData 获取用户数据
func (c *Client) GetUserData(key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok := c.UserData[key]
	return value, ok
}

// SetAuth 设置认证状态
func (c *Client) SetAuth(isAuth bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.IsAuth = isAuth
}

// GetAuth 获取认证状态
func (c *Client) GetAuth() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.IsAuth
}

// Close 关闭客户端连接
func (c *Client) Close() {
	c.cancel()
	c.Conn.Close()
}

// ReadPump 读取消息
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512 * 1024) // 512KB
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			messageType, data, err := c.Conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket错误: %v", err)
				}
				if c.Hub.OnDisconnect != nil {
					c.Hub.OnDisconnect(c, err)
				}
				return
			}

			// 如果设置了认证回调且客户端未认证，先进行认证
			if c.Hub.OnAuth != nil && !c.IsAuth {
				isAuth, err := c.Hub.OnAuth(c, messageType, data)
				if err != nil {
					log.Printf("认证错误: %v", err)
					c.Close()
					return
				}
				c.SetAuth(isAuth)

				// 如果认证失败，关闭连接
				if !isAuth {
					log.Printf("客户端 %s 认证失败，关闭连接", c.ID)
					c.Close()
					return
				}
				continue
			}

			// 如果需要认证但未认证，忽略消息
			if c.Hub.OnAuth != nil && !c.IsAuth {
				log.Printf("客户端 %s 未认证，忽略消息", c.ID)
				continue
			}

			// 触发消息回调
			if c.Hub.OnMessage != nil {
				if err := c.Hub.OnMessage(c, messageType, data); err != nil {
					log.Printf("消息处理错误: %v", err)
				}
			}
		}
	}
}

// WritePump 写入消息
func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 添加队列中的其他消息
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// WebSocketServer WebSocket服务器
type WebSocketServer struct {
	Hub      *Hub
	upgrader websocket.Upgrader
}

// NewWebSocketServer 创建WebSocket服务器
func NewWebSocketServer() *WebSocketServer {
	return &WebSocketServer{
		Hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 在生产环境中应该进行适当的Origin检查
			},
		},
	}
}

// SetCheckOrigin 设置跨域检查函数
func (s *WebSocketServer) SetCheckOrigin(checkOrigin func(r *http.Request) bool) {
	s.upgrader.CheckOrigin = checkOrigin
}

// HandleWebSocket 处理WebSocket连接
func (s *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket升级失败:", err)
		return
	}

	// 生成客户端ID（可以从请求中获取或自定义生成策略）
	clientID := s.generateClientID(r)

	client := NewClient(clientID, conn, s.Hub)
	client.Hub.register <- client

	// 启动读写协程
	go client.WritePump()
	go client.ReadPump()
}

// generateClientID 生成客户端ID（可以自定义实现）
func (s *WebSocketServer) generateClientID(r *http.Request) string {
	// 可以从URL参数、Header或其他方式获取ID
	if id := r.URL.Query().Get("id"); id != "" {
		return id
	}
	// 默认使用时间戳生成ID
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}

// Start 启动服务器
func (s *WebSocketServer) Start(addr string) error {
	go s.Hub.Run()

	http.HandleFunc("/ws", s.HandleWebSocket)

	log.Printf("WebSocket服务器启动在 %s", addr)
	return http.ListenAndServe(addr, nil)
}

// StartWithMux 使用自定义路由启动服务器
func (s *WebSocketServer) StartWithMux(addr string, mux *http.ServeMux) error {
	go s.Hub.Run()

	mux.HandleFunc("/ws", s.HandleWebSocket)

	log.Printf("WebSocket服务器启动在 %s", addr)
	return http.ListenAndServe(addr, mux)
}
