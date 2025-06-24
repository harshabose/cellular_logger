package cellularlog

import (
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/harshabose/cellular_localisation_logging/internal/multierr"
)

type LogEntry struct {
	Index        uint64                 `json:"index"`
	MessageType  string                 `json:"message_type"`
	MessageID    interface{}            `json:"message_id,omitempty"`
	Success      bool                   `json:"success"`
	Data         interface{}            `json:"data,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	RequestTime  time.Time              `json:"request_time,omitempty"`
	ResponseTime time.Time              `json:"response_time,omitempty"`
	Duration     time.Duration          `json:"duration,omitempty"`
}

type JSONWriter struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

func NewJSONWriter(filename string) (*JSONWriter, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	return &JSONWriter{
		file:    file,
		encoder: json.NewEncoder(file),
	}, nil
}

func (w *JSONWriter) Write(entries []LogEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, entry := range entries {
		if err := w.encoder.Encode(entry); err != nil {
			return fmt.Errorf("failed to write JSON entry: %w", err)
		}
	}

	return nil
}

func (w *JSONWriter) Close() error {
	return w.file.Close()
}

type CSVWriter struct {
	file   *os.File
	writer *csv.Writer
	mu     sync.Mutex
	header bool
}

func NewCSVWriter(filename string) (*CSVWriter, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	return &CSVWriter{
		file:   file,
		writer: csv.NewWriter(file),
	}, nil
}

func (w *CSVWriter) Write(entries []LogEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.header {
		header := []string{
			"index", "message_type", "message_id", "timestamp",
			"success", "data", "error", "request_time",
			"response_time", "duration_ms",
		}
		if err := w.writer.Write(header); err != nil {
			return err
		}
		w.header = true
	}

	for _, entry := range entries {
		record := []string{
			fmt.Sprintf("%d", entry.Index),
			entry.MessageType,
			fmt.Sprintf("%v", entry.MessageID),
			fmt.Sprintf("%t", entry.Success),
			fmt.Sprintf("%v", entry.Data),
			entry.Error,
			entry.RequestTime.Format(time.RFC3339Nano),
			entry.ResponseTime.Format(time.RFC3339Nano),
			fmt.Sprintf("%.2f", float64(entry.Duration.Nanoseconds())/1e6),
		}
		if err := w.writer.Write(record); err != nil {
			return err
		}
	}

	w.writer.Flush()
	return w.writer.Error()
}

func (w *CSVWriter) Close() error {
	w.writer.Flush()
	return w.file.Close()
}

type MultiWriter struct {
	writers []Writer
}

func NewMultiWriter(writers ...Writer) *MultiWriter {
	return &MultiWriter{writers: writers}
}

func (w *MultiWriter) Write(entries []LogEntry) error {
	var err error
	for _, writer := range w.writers {
		if e := writer.Write(entries); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return err
}

func (w *MultiWriter) Close() error {
	var err error
	for _, writer := range w.writers {
		if e := writer.Close(); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return err
}

type BinaryWriter struct {
	file *os.File
	mu   sync.Mutex
}

func NewBinaryWriter(filename string) (*BinaryWriter, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	return &BinaryWriter{file: file}, nil
}

func (w *BinaryWriter) Write(entries []LogEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}

		length := uint32(len(data))
		if err := binary.Write(w.file, binary.LittleEndian, length); err != nil {
			return err
		}
		if _, err := w.file.Write(data); err != nil {
			return err
		}
	}

	return nil
}

func (w *BinaryWriter) Close() error {
	return w.file.Close()
}

func FlattenStruct(v interface{}) map[string]string {
	result := make(map[string]string)
	flattenValue(reflect.ValueOf(v), "", result)
	return result
}

func flattenValue(v reflect.Value, prefix string, result map[string]string) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			result[prefix] = ""
			return
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			if t, ok := v.Interface().(time.Time); ok {
				result[prefix] = t.Format(time.RFC3339Nano)
			}
			return
		}

		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			if d, ok := v.Interface().(time.Duration); ok {
				result[prefix] = fmt.Sprintf("%.2f", float64(d.Nanoseconds())/1e6) // milliseconds
			}
			return
		}

		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			fieldValue := v.Field(i)

			if !fieldValue.CanInterface() {
				continue
			}

			fieldName := field.Name
			fullName := fieldName
			if prefix != "" {
				fullName = prefix + "_" + fieldName
			}

			flattenValue(fieldValue, fullName, result)
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			indexName := fmt.Sprintf("%s_%d", prefix, i)
			flattenValue(v.Index(i), indexName, result)
		}

	case reflect.Map:
		for _, key := range v.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			mapName := fmt.Sprintf("%s_%s", prefix, keyStr)
			flattenValue(v.MapIndex(key), mapName, result)
		}

	default:
		result[prefix] = formatValue(v)
	}
}

func formatValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.String:
		return v.String()
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}
