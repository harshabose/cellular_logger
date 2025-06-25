package main

import (
	"fmt"
	"go/format"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/bluenviron/gomavlib/v3/pkg/dialects/all"
)

func main() {
	if err := generateRegistry(); err != nil {
		fmt.Printf("Error generating registry: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Registry generated successfully!")
}

func generateRegistry() error {
	messages := all.Dialect.Messages

	var builder strings.Builder

	builder.WriteString(`package main

import (
	"context"

	"github.com/bluenviron/gomavlib/v3/pkg/dialects/all"
	"github.com/harshabose/cellular_localisation_logging"
	"github.com/harshabose/cellular_localisation_logging/pkg/mavlink"
)

var mavlinkRegistry = map[string]func(context.Context) cellularlog.Message{
`)

	type messageInfo struct {
		name     string
		typeName string
		id       uint32
	}

	var messageList []messageInfo

	for _, msg := range messages {
		msgType := reflect.TypeOf(msg)
		if msgType.Kind() == reflect.Ptr {
			msgType = msgType.Elem()
		}

		typeName := msgType.Name()

		name := extractMessageName(typeName)

		messageList = append(messageList, messageInfo{
			name:     name,
			typeName: typeName,
			id:       msg.GetID(),
		})
	}

	sort.Slice(messageList, func(i, j int) bool {
		return messageList[i].name < messageList[j].name
	})

	for _, msg := range messageList {
		builder.WriteString(fmt.Sprintf(`	"%s": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*all.%s](ctx)
	},
`, msg.name, msg.typeName))
	}

	builder.WriteString(`}

// Auto-generated ID to name mapping for reference
var messageIDToName = map[uint32]string{
`)

	sort.Slice(messageList, func(i, j int) bool {
		return messageList[i].id < messageList[j].id
	})

	for _, msg := range messageList {
		builder.WriteString(fmt.Sprintf(`	%d: "%s", // %s
`, msg.id, msg.name, msg.typeName))
	}

	builder.WriteString(`}

// Auto-generated function to create message by ID
func createMAVLinkMessageByID(id uint32, ctx context.Context) (cellularlog.Message, error) {
	name, exists := messageIDToName[id]
	if !exists {
		return nil, fmt.Errorf("unsupported message ID: %d", id)
	}
	
	factory, exists := mavlinkRegistry[name]
	if !exists {
		return nil, fmt.Errorf("message %s (ID: %d) not implemented in registry", name, id)
	}
	
	return factory(ctx), nil
}
`)

	formatted, err := format.Source([]byte(builder.String()))
	if err != nil {
		return fmt.Errorf("failed to format generated code: %w", err)
	}

	if err := os.WriteFile("generated_registry.go", formatted, 0644); err != nil {
		return fmt.Errorf("failed to write generated code: %w", err)
	}

	return nil
}

func extractMessageName(typeName string) string {
	if !strings.HasPrefix(typeName, "Message") {
		return typeName
	}

	name := typeName[7:]

	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(name[i-1])
			var next rune
			if i+1 < len(name) {
				next = rune(name[i+1])
			}

			if (prev >= 'a' && prev <= 'z') || (next >= 'a' && next <= 'z') {
				result.WriteRune('_')
			}
		}
		result.WriteRune(r)
	}

	return strings.ToUpper(result.String())
}
