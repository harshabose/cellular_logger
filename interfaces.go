package cellularlog

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/sets/hashset"

	"github.com/harshabose/cellular_localisation_logging/internal/multierr"
)

type Requester interface {
	Process(Message) (LogEntry, error)
}

type Message interface {
	Request(*Processor) (LogEntry, error)
	Process(Requester) (LogEntry, error)
	GetType() string
	GetAllEntries() []LogEntry
}

type Writer interface {
	Write(entries []LogEntry) error
	io.Closer
}

type Processor struct {
	Mavlink Requester
	AT      Requester

	messages *hashset.Set[Message]
	interval time.Duration
	writer   Writer

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	wg     sync.WaitGroup
	mux    sync.RWMutex

	logBatchSize int
	logBuffer    []LogEntry
	logMux       sync.Mutex
}

func NewProcessor(ctx context.Context, interval time.Duration, writer Writer, buffsize int, messages ...Message) *Processor {
	ctx2, cancel := context.WithCancel(ctx)

	p := &Processor{
		messages:     hashset.New(messages...),
		writer:       writer,
		interval:     interval,
		ctx:          ctx2,
		cancel:       cancel,
		logBatchSize: buffsize,
		logBuffer:    make([]LogEntry, 0, buffsize),
	}

	return p
}

func (p *Processor) AddMessage(m Message) {
	p.mux.Lock()
	defer p.mux.Unlock()

	p.messages.Add(m)
}

func (p *Processor) RemoveMessage(m Message) {
	p.mux.Lock()
	defer p.mux.Unlock()

	p.messages.Remove(m)
}

func (p *Processor) Start() {
	p.wg.Add(1)
	go p.loop()
}

func (p *Processor) loop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Periodic log flushing
	logTicker := time.NewTicker(5 * time.Second)
	defer logTicker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			p.flushLogs()
			return
		case <-ticker.C:
			if err := p.request(); err != nil {
				fmt.Printf("error processing: %v. Continuing...\n", err)
				continue
			}
		case <-logTicker.C:
			p.flushLogs()
		}
	}
}

func (p *Processor) request() error {
	messages := p.getMessages()

	var err error
	for _, message := range messages {
		log, e := message.Request(p)
		if e != nil {
			err = multierr.Append(err, e)
		}

		p.addLogEntry(log)
	}

	return err
}

func (p *Processor) getMessages() []Message {
	p.mux.RLock()
	defer p.mux.RUnlock()

	return p.messages.Values()
}

func (p *Processor) addLogEntry(entry LogEntry) {
	p.logMux.Lock()
	defer p.logMux.Unlock()

	p.logBuffer = append(p.logBuffer, entry)

	if len(p.logBuffer) >= p.logBatchSize {
		p.flushLogsUnsafe()
	}
}

func (p *Processor) flushLogs() {
	p.logMux.Lock()
	defer p.logMux.Unlock()
	p.flushLogsUnsafe()
}

func (p *Processor) flushLogsUnsafe() {
	if len(p.logBuffer) == 0 {
		return
	}

	if err := p.writer.Write(p.logBuffer); err != nil {
		fmt.Printf("error writing logs: %v\n", err)
	}

	p.logBuffer = make([]LogEntry, 0, p.logBatchSize) // Reset buffer
}

func (p *Processor) Close() error {
	var err error

	p.once.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}

		p.wg.Wait()

		if p.writer != nil {
			if e := p.writer.Close(); e != nil {
				err = multierr.Append(err, fmt.Errorf("error closing writer: %w", e))
			}
		}
	})

	if err != nil {
		return fmt.Errorf("error while closing processor: %w", err)
	}

	return nil
}

// ========================
// HELPER FUNCTIONS
// ========================

func (p *Processor) GetSuccessRate(messageType string) float64 {
	messages := p.getMessages()

	var total, successful int
	for _, message := range messages {
		if message.GetType() == messageType {
			entries := message.GetAllEntries()
			total += len(entries)
			for _, entry := range entries {
				if entry.Success {
					successful++
				}
			}
		}
	}

	if total == 0 {
		return 0
	}
	return float64(successful) / float64(total) * 100
}

func (p *Processor) GetAverageResponseTime(messageType string) time.Duration {
	messages := p.getMessages()

	var totalDuration time.Duration
	var count int

	for _, message := range messages {
		if message.GetType() == messageType {
			entries := message.GetAllEntries()
			for _, entry := range entries {
				if entry.Success && entry.Duration > 0 {
					totalDuration += entry.Duration
					count++
				}
			}
		}
	}

	if count == 0 {
		return 0
	}
	return totalDuration / time.Duration(count)
}
