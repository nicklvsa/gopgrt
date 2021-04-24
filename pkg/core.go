package pkg

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

func (s *SocketCore) Run() {
	for {
		select {
		case client := <-s.Create:
			s.HandleCreateUser(client)
		case client := <-s.Destroy:
			s.HandleDestroyUser(client)
		}
	}
}

func (s *SocketCore) BroadcastAll(current *SocketClient, payload *SocketEvent) error {
	if payload == nil || payload.Payload == nil {
		return fmt.Errorf("payload cannot be nil")
	}

	for client, active := range s.Clients {
		if active {
			client.Data <- *payload
		}
	}

	return nil
}

func (s *SocketCore) EmitToClient(client *SocketClient, payload *SocketEvent) error {
	if payload == nil || payload.Payload == nil {
		return fmt.Errorf("payload cannot be nil")
	}

	client.Data <- *payload

	return nil
}

func (s *SocketCore) DestroyClient(client *SocketClient) error {
	if client.Data != nil {
		close(client.Data)
	}

	if err := client.Connection.Close(); err != nil {
		return err
	}

	delete(s.Clients, client)

	return nil
}

func (s *SocketCore) HandleCreateUser(client *SocketClient) {
	s.Clients[client] = true

	event := SocketEvent{
		Type: SocketEventTypeConnect,
		Payload: ConnectionPayload{
			UserID: &client.User.UserID,
		},
	}

	if err := s.HandleEvent(client, &event); err != nil {
		fmt.Println(fmt.Sprintf("[ERR] - %s", err.Error()))
	}
}

func (s *SocketCore) HandleDestroyUser(client *SocketClient) {
	if _, active := s.Clients[client]; active {
		event := SocketEvent{
			Type: SocketEventTypeDisconnect,
			Payload: ConnectionPayload{
				UserID: &client.User.UserID,
			},
		}

		if err := s.HandleEvent(client, &event); err != nil {
			fmt.Println(fmt.Sprintf("[ERR] - %s", err.Error()))
		}

		if err := s.DestroyClient(client); err != nil {
			fmt.Println(fmt.Sprintf("[ERR] - %s", err.Error()))
		}

		delete(s.Clients, client)
	}
}

func (s *SocketCore) HandleEvent(client *SocketClient, payload *SocketEvent) error {
	switch payload.Type {
	case SocketEventTypeConnect:
		if err := s.BroadcastAll(client, payload); err != nil {
			return err
		}
	case SocketEventTypeDisconnect:
		if err := s.BroadcastAll(client, payload); err != nil {
			return err
		}
	case SocketEventTypeSubscribe:
		var subscription SubscriptionPayload
		if err := UnmarshalInterface(payload.Payload, &subscription); err != nil {
			return err
		}

		if subscription.TableName == nil {
			return fmt.Errorf("table name required for subscription payload")
		}

		listenerChan := make(chan TriggerEvent)
		errChan := make(chan error)

		go client.Trigger.Listen(listenerChan, errChan)

		go func() {
			for {
				select {
				case listen := <-listenerChan:
					response := SocketEvent{
						Type:    SocketEventTypeSubscribeRespond,
						Payload: listen.Payload,
					}

					if err := s.EmitToClient(client, &response); err != nil {
						fmt.Printf("Error: %s\n", err.Error())
					}
				}
			}
		}()

		response := SocketEvent{
			Type: SocketEventTypeSubscribeRespond,
			Payload: MessagePayload{
				Message: fmt.Sprintf("You successfully subscribed to table %s", *subscription.TableName),
			},
		}

		if err := s.EmitToClient(client, &response); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid payload detected")
	}

	return nil
}

func (s *SocketCore) RegisterWriter(client *SocketClient) {
	ticker := time.NewTicker(SocketPingPeriod)

	defer func() {
		ticker.Stop()
		client.Connection.Close()
	}()

	for {
		select {
		case payload, ok := <-client.Data:
			client.Connection.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))

			encoded, err := json.Marshal(payload)
			if err != nil || !ok {
				client.Connection.WriteMessage(websocket.CloseMessage, EmptySocketBytes)
				return
			}

			writer, err := client.Connection.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			writer.Write(encoded)

			for i := 0; i < len(client.Data); i++ {
				data, err := json.Marshal(<-client.Data)
				if err != nil {
					return
				}

				writer.Write(data)
			}

			if err := writer.Close(); err != nil {
				return
			}
		case <-ticker.C:
			client.Connection.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
			if err := client.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *SocketCore) RegisterReader(client *SocketClient) {
	defer func() {
		client.Core.Destroy <- client
		client.Connection.Close()
	}()

	client.Connection.SetReadLimit(SocketMaxMessageSize)
	client.Connection.SetReadDeadline(time.Now().Add(SocketPingAckTimeout))
	client.Connection.SetPongHandler(func(data string) error {
		client.Connection.SetReadDeadline(time.Now().Add(SocketPingAckTimeout))
		return nil
	})

	for {
		_, payload, err := client.Connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Println(fmt.Sprintf("[ERR] - %s", err.Error()))
			}

			break
		}

		var event SocketEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			fmt.Println(fmt.Sprintf("[ERR] - %s", err.Error()))
		}

		if s.HandleEvent(client, &event); err != nil {
			fmt.Println(fmt.Sprintf("[ERR] - %s", err.Error()))
		}
	}
}
