package AT

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/warthog618/modem/at"
	"github.com/warthog618/modem/serial"

	"github.com/harshabose/cellular_localisation_logging"
)

type AT struct {
	s    *serial.Port
	node *at.AT
}

func NewAT(device string, baud int, timeout time.Duration) (*AT, error) {
	s, err := serial.New(serial.WithPort(device), serial.WithBaud(baud))
	if err != nil {
		return nil, err
	}

	node := at.New(s, at.WithTimeout(timeout))
	if err := node.Init(); err != nil {
		return nil, err
	}

	return &AT{
		node: node,
	}, nil
}

func (r *AT) Process(messages cellularlog.Message) (cellularlog.LogEntry, error) {
	return messages.Process(r)
}

type Message struct {
	index    uint64
	messages []cellularlog.LogEntry
	cmd      string
	mux      sync.RWMutex
}

// NewMessage example
//
//	NewMessage("I")
//	NewMessage("+GCAP")
//	NewMessage("+CNMI=?")
func NewMessage(cmd string) *Message {
	return &Message{
		messages: make([]cellularlog.LogEntry, 0),
		cmd:      cmd,
	}
}

func (m *Message) Request(processor *cellularlog.Processor) (cellularlog.LogEntry, error) {
	return processor.AT.Process(m)
}

func (m *Message) Process(requester cellularlog.Requester) (cellularlog.LogEntry, error) {
	defer func() { m.index++ }()

	requestTime := time.Now()
	log := cellularlog.LogEntry{
		Index:       m.index,
		MessageType: m.GetType(),
		Success:     false,
		RequestTime: requestTime,
	}

	r, ok := requester.(*AT)
	if !ok {
		log.Error = "errors interface mismatch"

		m.add(log)
		return log, errors.New("error interface mismatch")
	}

	data, err := r.node.Command(m.cmd)
	if err != nil {
		log.Error = fmt.Errorf("error while sending AT commands: %w", err).Error()

		m.add(log)
		return log, fmt.Errorf("error while sending AT commands: %w", err)
	}

	log.Success = true
	log.Data = data
	log.ResponseTime = time.Now()
	log.Duration = log.ResponseTime.Sub(log.RequestTime)

	m.add(log)

	return log, nil
}

func (m *Message) add(log cellularlog.LogEntry) {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.messages = append(m.messages, log)
}

func (m *Message) GetType() string {
	return fmt.Sprintf("at-%s", m.cmd)
}

func (m *Message) GetAllEntries() []cellularlog.LogEntry {
	m.mux.RLock()
	defer m.mux.RUnlock()

	return m.messages
}
