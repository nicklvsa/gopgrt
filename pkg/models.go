package pkg

import (
	"github.com/gorilla/websocket"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type SocketEventType string

const (
	SocketEventTypeConnect          SocketEventType = "connect"
	SocketEventTypeDisconnect       SocketEventType = "disconnect"
	SocketEventTypeSubscribe        SocketEventType = "subscribe"
	SocketEventTypeUnsubscribe      SocketEventType = "unsubscribe"
	SocketEventTypeSubscribeRespond SocketEventType = "subscription:response"
)

type SocketEvent struct {
	Type    SocketEventType `json:"payload_type"`
	Payload interface{}     `json:"payload"`
}

type SocketUser struct {
	UserID string `json:"user_id"`
	Token  string `json:"token"`
}

type SocketCore struct {
	isInitialized bool
	Clients       map[*SocketClient]bool
	Create        chan *SocketClient
	Destroy       chan *SocketClient
}

type SocketClient struct {
	Core              *SocketCore
	Connection        *websocket.Conn
	Data              chan SocketEvent
	User              *SocketUser
	Trigger           *Trigger
	Unsubscribe       map[string]chan int8
	TableSubscription map[string]func()
}

type ConnectionPayload struct {
	UserID *string `json:"user_id"`
}

type SubscriptionPayload struct {
	TableName *string  `json:"table_name"`
	Columns   []string `json:"columns"`
}

type UnsubscribePayload struct {
	TableName *string `json:"table_name"`
}

type MessagePayload struct {
	Message string `json:"message"`
}

type PGConnection struct {
	DBName   string
	Host     string
	Port     int
	Username string
	Password string
}

type Trigger struct {
	Name                *string
	Channel             *string
	Connection          *gorm.DB
	DSN                 string
	AllowExternalListen bool
}

type TriggerEvent struct {
	TableName string
	Payload   TriggerPayload
}

type TriggerPayload struct {
	Table  string                 `json:"table"`
	Action string                 `json:"action"`
	Data   map[string]interface{} `json:"data"`
}

type TriggerError struct {
	EventType pq.ListenerEventType
	Error     error
}
