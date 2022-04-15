package irc

import (
	"fmt"
	"strings"
)

var errInvalidTags = fmt.Errorf("invalid tags")

type MessageType string

const (
	MessageTypePrivmsg         = "PRIVMSG"
	MessageTypeNotice          = "NOTICE"
	MessageTypePing            = "PING"
	MessageTypePong            = "PONG"
	MessageTypeCap             = "CAP"
	MessageType001             = "001"
	MessageType002             = "002"
	MessageType003             = "003"
	MessageType004             = "004"
	MessageType353             = "353"
	MessageType366             = "366"
	MessageType372             = "372"
	MessageType375             = "375"
	MessageType376             = "376"
	MessageTypeUnknown         = "UNKNOWN"
	MessageTypeJoin            = "JOIN"
	MessageTypePart            = "PART"
	MessageTypeGlobalUserState = "GLOBALUSERSTATE"
	MessageTypeWhisper         = "WHISPER"
	MessageTypeRoomState       = "ROOMSTATE"
	MessageTypeUserState       = "USERSTATE"
	MessageTypeClearChat       = "CLEARCHAT"
	MessageTypeReconnect       = "RECONNECT"
)

func ToMessageType(s string) MessageType {
	m := MessageType(s)
	switch m {
	case MessageTypePrivmsg,
		MessageTypeNotice,
		MessageTypePing,
		MessageTypePong,
		MessageTypeCap,
		MessageType001,
		MessageType002,
		MessageType003,
		MessageType004,
		MessageType353,
		MessageType366,
		MessageType372,
		MessageType375,
		MessageType376,
		MessageTypeJoin,
		MessageTypePart,
		MessageTypeWhisper,
		MessageTypeGlobalUserState,
		MessageTypeRoomState,
		MessageTypeUserState,
		MessageTypeClearChat,
		MessageTypeReconnect:
		return m
	}

	return MessageTypeUnknown
}

type Message struct {
	Raw     string
	Source  string
	Type    MessageType
	Channel string
	User    string
	Content string
	Tags    map[string]string
}

func ParseMessage(line string) (Message, error) {
	switch line[:4] {
	case "PING":
		return Message{
			Raw:    line,
			Source: line[6:],
			Type:   MessageTypePing,
		}, nil
	case "PONG":
		return Message{
			Raw:    line,
			Source: line[6:],
			Type:   MessageTypePing,
		}, nil
	}
	m := Message{
		Type: MessageTypeUnknown,
		Raw:  line,
	}

	tags, idx, err := ParseTags(line)
	if err != nil {
		return m, err
	}

	m.Tags = tags
	line = line[idx:]

	// parse the tmi source
	splits := strings.SplitN(line, " ", 3)
	if len(splits) < 2 || len(splits[0]) == 0 {
		return m, nil
	}

	m.Source = splits[0][1:]
	idx = strings.IndexByte(m.Source, '!')
	if idx != -1 {
		m.User = m.Source[:idx]
	}
	m.Type = ToMessageType(splits[1])
	switch m.Type {
	case MessageTypeUnknown, MessageTypeGlobalUserState:
		return m, nil
	default:
		line = splits[2]
		contentIdx := strings.IndexByte(line, ':')
		if contentIdx != -1 {
			m.Content = line[contentIdx+1:]
			line = line[:contentIdx]
		}

		m.Channel = strings.TrimSpace(line)
	}

	return m, nil
}

func ParseTags(msg string) (map[string]string, int, error) {
	i := 0
	if msg[i] != '@' {
		return nil, 0, nil
	}

	i++
	s := i

	tags := map[string]string{}
	var key string

	for i < len(msg) {
		switch msg[i] {
		case '=':
			if key != "" || s == i {
				return nil, 0, errInvalidTags
			}
			key = msg[s:i]
			s = i + 1
		case ';':
			if key == "" {
				return nil, 0, errInvalidTags
			}
			tags[key] = msg[s:i]
			key = ""
			i++
			s = i
		case ' ':
			if key == "" {
				return nil, 0, errInvalidTags
			}
			tags[key] = msg[s:i]
			return tags, i + 1, nil
		}
		i++
	}

	return tags, i, nil
}
