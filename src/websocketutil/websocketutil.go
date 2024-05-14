// Package websocketutil contains methods for interacting with websocket connections.
package websocketutil

import (
	"errors"

	"github.com/aws/session-manager-plugin/src/log"
	"github.com/gorilla/websocket"
)

// IWebsocketUtil is the interface for the websocketutil.
type IWebsocketUtil interface {
	OpenConnection(url string) (*websocket.Conn, error)
	CloseConnection(ws websocket.Conn) error
}

// WebsocketUtil struct provides functionality around creating and maintaining websockets.
type WebsocketUtil struct {
	dialer *websocket.Dialer
}

// NewWebsocketUtil is the factory function for websocketutil.
func NewWebsocketUtil(dialerInput *websocket.Dialer) *WebsocketUtil {

	var websocketUtil *WebsocketUtil

	if dialerInput == nil {
		websocketUtil = &WebsocketUtil{
			dialer: websocket.DefaultDialer,
		}
	} else {
		websocketUtil = &WebsocketUtil{
			dialer: dialerInput,
		}
	}

	return websocketUtil
}

// OpenConnection opens a websocket connection provided an input url.
func (u *WebsocketUtil) OpenConnection(url string) (*websocket.Conn, error) {

	log.Infof("Opening websocket connection to: %s", url)

	conn, _, err := u.dialer.Dial(url, nil)
	if err != nil {
		log.Errorf("Failed to dial websocket: %s", err.Error())
		return nil, err
	}

	log.Infof("Successfully opened websocket connection to: %s", url)

	return conn, err
}

// CloseConnection closes a websocket connection given the Conn object as input.
func (u *WebsocketUtil) CloseConnection(ws *websocket.Conn) error {

	if ws == nil {
		return errors.New("websocket conn object is nil")
	}

	log.Debugf("Closing websocket connection to: %s", ws.RemoteAddr().String())

	err := ws.Close()
	if err != nil {
		log.Errorf("Failed to close websocket: %s", err.Error())
		return err
	}

	log.Debugf("Successfully closed websocket connection to: %s", ws.RemoteAddr().String())

	return nil
}
