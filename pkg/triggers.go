package pkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewTrigger(dsn string, name, channel string) *Trigger {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err.Error())
	}

	InitCore()

	return &Trigger{
		Name:       &name,
		Channel:    &channel,
		DSN:        dsn,
		Connection: db,
	}
}

func (t *Trigger) Create() error {
	if t.Channel == nil || t.Name == nil || t.Connection == nil {
		return fmt.Errorf("trigger must be initialized before registering")
	}

	triggerStmt := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS TRIGGER AS $$
			DECLARE
				data json;
				push json;

			BEGIN

				IF (TG_OP = 'DELETE') THEN 
					data = row_to_json(OLD);
				ELSE
					data = row_to_json(NEW);
				END IF;

				push = json_build_object(
					'table', TG_TABLE_NAME,
					'action', TG_OP,
					'data', data
				);

				PERFORM pg_notify('%s', push::text);

				RETURN NULL;
			END;
		$$ LANGUAGE plpgsql;
	`, *t.Name, *t.Channel)

	if err := t.Connection.Exec(triggerStmt).Error; err != nil {
		return err
	}

	return nil
}

func (t *Trigger) RegisterTable(tableName string) error {
	if t.Channel == nil || t.Name == nil || t.Connection == nil {
		return fmt.Errorf("trigger must be initialized before registering")
	}

	triggerExists := fmt.Sprintf(`
		SELECT pg_get_triggerdef(t.oid) AS "trigger_decl"
		FROM pg_trigger t WHERE NOT tgisinternal AND
		tgrelid='%s'::regclass;
	`, tableName)

	data := make(map[string]interface{})
	if err := t.Connection.Raw(triggerExists).Scan(&data).Error; err != nil {
		return err
	}

	if decl, ok := data["trigger_decl"]; !ok || decl.(string) == "" {
		listenStmt := fmt.Sprintf(`
			CREATE TRIGGER %s_notify_event AFTER
			INSERT OR UPDATE OR DELETE ON %s FOR
			EACH ROW EXECUTE PROCEDURE %s();
		`, tableName, tableName, *t.Name)

		if err := t.Connection.Exec(listenStmt).Error; err != nil {
			return err
		}
	}

	return nil
}

func (t *Trigger) Serve(port int, allowedOrigins []string) {
	r := gin.Default()
	buffer := 1024

	r.Any("/ws/:token", func(c *gin.Context) {
		token := c.Param("token")

		upgrader := websocket.Upgrader{
			ReadBufferSize:  buffer,
			WriteBufferSize: buffer,
		}

		upgrader.CheckOrigin = func(r *http.Request) bool {
			if allowedOrigins == nil {
				return true
			}

			for _, allowed := range allowedOrigins {
				if r.URL.String() == allowed {
					return true
				}
			}

			return false
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		if err := RegisterConnection(conn, token, t); err != nil {
			return
		}
	})

	r.Run(fmt.Sprintf(":%d", port))
}

func (t *Trigger) Listen(dataRecv chan<- TriggerEvent, errRecv chan<- error) {
	handleErr := func(event pq.ListenerEventType, err error) {
		if err != nil {
			errRecv <- err
		}
	}

	listener := pq.NewListener(t.DSN, 10*time.Second, time.Minute, handleErr)
	if err := listener.Listen(*t.Channel); err != nil {
		errRecv <- err
		return
	}

	for {
		select {
		case recv := <-listener.Notify:
			var data bytes.Buffer
			if err := json.Indent(&data, []byte(recv.Extra), "", "\t"); err != nil {
				errRecv <- err
				return
			}

			var payload TriggerPayload
			if err := json.Unmarshal(data.Bytes(), &payload); err != nil {
				errRecv <- err
				return
			}

			dataRecv <- TriggerEvent{
				TableName: *t.Name,
				Payload:   payload,
			}
		case <-time.After(60 * time.Second):
			go func() {
				if err := listener.Ping(); err != nil {
					errRecv <- err
				}
			}()
		}
	}
}
