package irc

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/SevenTV/Common/utils"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

var errReconnect = fmt.Errorf("reconnect")

const (
	pingSource  = "7tv-chat-edge"
	pingMessage = "PING :" + pingSource + "\r\n"
)

type loginDetails struct {
	username string
	oauth    string
}

type Client struct {
	conn            net.Conn
	openConnLimiter func()
	err             *errorChan
	writeCh         chan string

	login loginDetails

	pingSent  bool
	connected bool
	mtx       sync.Mutex

	onMessage   func(Message)
	onReconnect func()
	onConnect   func()
}

func New(username string, oauth string, openConnLimiter func()) *Client {
	if username == "" || oauth == "" {
		panic(fmt.Errorf("bad username or oauth: %s", username))
	}
	return &Client{
		err:             &errorChan{},
		writeCh:         make(chan string, 100),
		openConnLimiter: openConnLimiter,
		login: loginDetails{
			username: username,
			oauth:    oauth,
		},
	}
}

func (c *Client) Connect(ctx context.Context) error {
	for {
		err := c.init(ctx)
		c.Shutdown()
		switch err {
		case errReconnect, nil:
			if c.onReconnect != nil {
				c.onReconnect()
			}
			continue
		default:
			return err
		}
	}
}

func (c *Client) Shutdown() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Client) SetOAuth(oauth string) {
	c.login.oauth = oauth
}

func (c *Client) init(ctx context.Context) error {
	utils.EmptyChannel(c.writeCh)

	lCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.openConnLimiter()

	conn, err := tls.Dial("tcp", "irc.chat.twitch.tv:6697", &tls.Config{})
	if err != nil {
		return err
	}

	c.conn = conn

	_, err = conn.Write([]byte("CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands\r\n"))
	if err != nil {
		return err
	}

	_, err = c.conn.Write([]byte("PASS " + c.login.oauth + "\r\n"))
	if err != nil {
		return err
	}
	_, err = c.conn.Write([]byte("NICK " + c.login.username + "\r\n"))
	if err != nil {
		return err
	}

	c.err.Reset()

	go c.read(conn)
	go c.write(conn)
	go c.pinger()

	go func() {
		select {
		case <-ctx.Done():
			c.err.Error(ctx.Err())
		case <-lCtx.Done():
			if ctx.Err() != nil {
				c.err.Error(ctx.Err())
			}
			return
		}
	}()

	err = <-c.err.ch

	c.mtx.Lock()
	c.connected = true
	c.mtx.Unlock()

	return err
}

func (c *Client) read(conn net.Conn) {
	msgBuffer := bytes.NewBuffer(nil)
	defer func() {
		_ = c.conn.Close()
		zap.S().Errorw("read to conn failed",
			"error", c.err,
			"data", msgBuffer.String(),
		)
	}()

	// stores last 2048 bytes of messages
	tp := textproto.NewReader(bufio.NewReader(conn))
	for {
		line, err := tp.ReadLine()
		if err != nil {
			c.err.Error(fmt.Errorf("read failed: %e", err))
			return
		}

		msgBuffer.WriteString(line + "\n")
		if msgBuffer.Len() > 2048 {
			msgBuffer.Truncate(msgBuffer.Len() - 2048)
		}

		msg, err := ParseMessage(line)
		if err != nil {
			continue
		}

		switch msg.Type {
		case MessageType376:
			c.mtx.Lock()
			c.connected = true
			c.mtx.Unlock()
			if c.onConnect != nil {
				c.onConnect()
			}
		case MessageTypePing:
			if err := c.Write(fmt.Sprintf("PONG :%s\r\n", msg.Source)); err != nil {
				c.err.Error(err)
				return
			}
		case MessageTypePong:
			if msg.Content == pingSource {
				c.pingSent = false
			}
		}
		if c.onMessage != nil {
			c.onMessage(msg)
		}
		if msg.Type == MessageTypeReconnect {
			c.err.Error(errReconnect)
			return
		}
	}
}

func (c *Client) write(conn net.Conn) {
	defer func() {
		_ = c.conn.Close()
	}()

	for msg := range c.writeCh {
		_, err := c.conn.Write([]byte(msg))
		if err != nil {
			c.err.Error(fmt.Errorf("write failed: %e", err))
			return
		}
	}
}

func (c *Client) pinger() {
	defer func() {
		c.err.Error(errReconnect)
		_ = c.conn.Close()
	}()

	tick := time.NewTicker(time.Minute * 3)
	for range tick.C {
		if c.pingSent {
			zap.S().Errorw("Ping didnt respond in time")
			return
		}
		c.writeCh <- pingMessage
		c.pingSent = true
	}
}

func (c *Client) SetOnMessage(f func(Message)) {
	c.onMessage = f
}

func (c *Client) SetOnReconnect(f func()) {
	c.onReconnect = f
}

func (c *Client) SetOnConnect(f func()) {
	c.onConnect = f
}

func (c *Client) IsConnected() bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	return c.connected
}

func (c *Client) Write(msg string) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}
	if !strings.HasSuffix(msg, "\r\n") {
		c.writeCh <- msg + "\r\n"
		return nil
	}

	c.writeCh <- msg
	return nil
}
