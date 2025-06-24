package mavlink

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/gomavlib/v3"
	"github.com/bluenviron/gomavlib/v3/pkg/dialect"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/ardupilotmega"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/common"
	"github.com/bluenviron/gomavlib/v3/pkg/message"

	"github.com/harshabose/cellular_localisation_logging"
)

type Mavlink struct {
	node    *gomavlib.Node
	timeout time.Duration
}

func NewMavlink(device string, baud int, timeout time.Duration, dialect *dialect.Dialect, version gomavlib.Version) (*Mavlink, error) {
	r := &Mavlink{
		node: &gomavlib.Node{
			Endpoints: []gomavlib.EndpointConf{
				gomavlib.EndpointSerial{
					Device: device,
					Baud:   baud,
				},
			},
			Dialect:     dialect,
			OutVersion:  version,
			OutSystemID: 10,
		},
		timeout: timeout,
	}

	if err := r.node.Initialize(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Mavlink) Process(messages cellularlog.Message) (cellularlog.LogEntry, error) {
	return messages.Process(r)
}

type Message[T message.Message] struct {
	index    uint64
	messages []cellularlog.LogEntry
	id       uint32
	ctx      context.Context
	mux      sync.RWMutex
}

func NewMessage[T message.Message](ctx context.Context) *Message[T] {
	empty := new(T)

	return &Message[T]{
		messages: make([]cellularlog.LogEntry, 0),
		id:       (*empty).GetID(),
		ctx:      ctx,
	}
}

func (m *Message[T]) Request(processor *cellularlog.Processor) (cellularlog.LogEntry, error) {
	return processor.Mavlink.Process(m)
}

func (m *Message[T]) Process(requester cellularlog.Requester) (cellularlog.LogEntry, error) {
	defer func() { m.index++ }()

	requestTime := time.Now()
	log := cellularlog.LogEntry{
		Index:       m.index,
		MessageType: m.GetType(),
		Success:     false,
		RequestTime: requestTime,
	}

	r, ok := requester.(*Mavlink)
	if !ok {
		log.Error = "errors interface mismatch"

		m.add(log)
		return log, errors.New("error interface mismatch")
	}

	if err := r.node.WriteMessageAll(&ardupilotmega.MessageCommandLong{
		TargetSystem:    1,
		TargetComponent: 0,
		Command:         common.MAV_CMD_REQUEST_MESSAGE,
		Confirmation:    0,
		Param1:          float32(m.id),
	}); err != nil {
		log.Error = err.Error()

		m.add(log)
		return log, err
	}

	ctx, cancel := context.WithTimeout(m.ctx, r.timeout)
	defer cancel()

	for {
		select {
		case <-m.ctx.Done():
			log.Error = "context cancelled"

			m.add(log)
			return log, nil
		case <-ctx.Done():
			log.Error = "request timeout"

			m.add(log)
			return log, ctx.Err()
		case event, ok := <-r.node.Events():
			if !ok {
				log.Error = "errors interface mismatch"

				m.add(log)
				return log, errors.New("error interface mismatch")
			}

			if frm, ok := event.(*gomavlib.EventFrame); ok {
				msg, ok := frm.Message().(T)
				if !ok {
					continue
				}

				log.Success = true
				log.Data = msg
				log.ResponseTime = time.Now()
				log.Duration = log.ResponseTime.Sub(log.RequestTime)

				m.add(log)

				return log, nil
			}
		}
	}
}

func (m *Message[T]) add(log cellularlog.LogEntry) {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.messages = append(m.messages, log)
}

func (m *Message[T]) GetType() string {
	return fmt.Sprintf("mavlink-%d", m.id)
}

func (m *Message[T]) GetAllEntries() []cellularlog.LogEntry {
	m.mux.RLock()
	defer m.mux.RUnlock()

	return m.messages
}
