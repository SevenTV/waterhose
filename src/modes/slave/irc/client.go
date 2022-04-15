package irc

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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
	conn    net.Conn
	err     *errorChan
	writeCh chan string

	login loginDetails

	pingSent bool

	onMessage   func(Message)
	onReconnect func()
	onConnect   func()
}

func New(username string, oauth string) *Client {
	logrus.Info("new client")
	if username == "" || oauth == "" {
		panic(fmt.Errorf("bad username or oauth: %s", username))
	}
	return &Client{
		err:     &errorChan{},
		writeCh: make(chan string, 100),
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
	_ = c.conn.Close()
}

func (c *Client) SetOAuth(oauth string) {
	c.login.oauth = oauth
}

func (c *Client) init(ctx context.Context) error {
	lCtx, cancel := context.WithCancel(ctx)
	defer cancel()

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

	return <-c.err.ch
}

func (c *Client) read(conn net.Conn) {
	defer func() {
		c.err.Error(errReconnect)
		_ = c.conn.Close()
		log.Println("read failed")
	}()

	tp := textproto.NewReader(bufio.NewReader(conn))

	for {
		line, err := tp.ReadLine()
		if err != nil {
			log.Println(err)
			return
		}

		msg, err := ParseMessage(line)
		if err != nil {
			continue
		}

		switch msg.Type {
		case MessageType376:
			if c.onConnect != nil {
				c.onConnect()
			}
		case MessageTypePing:
			c.Write(fmt.Sprintf("PONG :%s\r\n", msg.Source))
		case MessageTypePong:
			if msg.Content == pingSource {
				c.pingSent = false
			}
		}
		if c.onMessage != nil {
			c.onMessage(msg)
		}
		if msg.Type == MessageTypeReconnect {
			return
		}
	}
}

func (c *Client) write(conn net.Conn) {
	defer func() {
		c.err.Error(errReconnect)
		_ = c.conn.Close()

		log.Println("write failed")
	}()

	for msg := range c.writeCh {
		_, err := c.conn.Write([]byte(msg))
		if err != nil {
			return
		}
	}
}

func (c *Client) pinger() {
	defer func() {
		c.err.Error(errReconnect)
		_ = c.conn.Close()

		log.Println("ping failed")
	}()
	tick := time.NewTicker(time.Minute * 3)
	for range tick.C {
		if c.pingSent {
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

func (c *Client) Write(msg string) {
	if !strings.HasSuffix(msg, "\r\n") {
		c.writeCh <- msg + "\r\n"
		return
	}

	c.writeCh <- msg
}
