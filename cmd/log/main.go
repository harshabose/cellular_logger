package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bluenviron/gomavlib/v3"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/all"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/ardupilotmega"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/common"

	"github.com/harshabose/cellular_localisation_logging"
	"github.com/harshabose/cellular_localisation_logging/pkg/AT"
	"github.com/harshabose/cellular_localisation_logging/pkg/mavlink"
)

type Config struct {
	Messages     string
	OutputFormat string
	OutputFile   string
	BufferSize   int
	Interval     time.Duration

	// MAVLink specific
	MAVDevice  string
	MAVBaud    int
	MAVTimeout time.Duration

	// AT specific
	ATDevice  string
	ATBaud    int
	ATTimeout time.Duration

	// Utility flags
	ListMessages bool
	Verbose      bool
}

var mavlinkRegistry = map[string]func(context.Context) cellularlog.Message{
	// IMU Messages (for inertial navigation)
	"SCALED_IMU": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageScaledImu](ctx)
	},
	"SCALED_IMU2": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageScaledImu2](ctx)
	},
	"SCALED_IMU3": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageScaledImu3](ctx)
	},
	"RAW_IMU": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageRawImu](ctx)
	},

	// GPS Messages (for satellite-based positioning)
	"GPS_RAW_INT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageGpsRawInt](ctx)
	},
	"GPS2_RAW": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageGps2Raw](ctx)
	},
	"GPS_STATUS": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageGpsStatus](ctx)
	},

	// Position Messages (for fused position estimates)
	"GLOBAL_POSITION_INT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageGlobalPositionInt](ctx)
	},
	"LOCAL_POSITION_NED": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageLocalPositionNed](ctx)
	},

	// Attitude and Orientation (for complete pose estimation)
	"ATTITUDE": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageAttitude](ctx)
	},
	"ATTITUDE_QUATERNION": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageAttitudeQuaternion](ctx)
	},

	// Magnetometer Messages (for compass/heading data)
	"SCALED_PRESSURE": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageScaledPressure](ctx)
	},
	"MAG_CAL_REPORT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageMagCalReport](ctx)
	},

	// Navigation and Control Messages
	"NAV_CONTROLLER_OUTPUT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageNavControllerOutput](ctx)
	},
	"POSITION_TARGET_GLOBAL_INT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessagePositionTargetGlobalInt](ctx)
	},
	"POSITION_TARGET_LOCAL_NED": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessagePositionTargetLocalNed](ctx)
	},

	// System Status (for understanding system state)
	"SYS_STATUS": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageSysStatus](ctx)
	},
	"HEARTBEAT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageHeartbeat](ctx)
	},

	// EKF/Filter Status (for understanding fusion quality)
	"EKF_STATUS_REPORT": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageEkfStatusReport](ctx)
	},
	"AHRS": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageAhrs](ctx)
	},
	"AHRS2": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*ardupilotmega.MessageAhrs2](ctx)
	},

	// High-rate position data
	"HIGH_LATENCY": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageHighLatency](ctx)
	},
	"HIGH_LATENCY2": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageHighLatency2](ctx)
	},

	// Optical Flow (if available - for visual positioning)
	"OPTICAL_FLOW": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageOpticalFlow](ctx)
	},
	"OPTICAL_FLOW_RAD": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageOpticalFlowRad](ctx)
	},

	// Velocity and acceleration
	"LOCAL_POSITION_NED_SYSTEM_GLOBAL_OFFSET": func(ctx context.Context) cellularlog.Message {
		return mavlink.NewMessage[*common.MessageLocalPositionNedSystemGlobalOffset](ctx)
	},
}

func main() {
	config := parseFlags()

	if config.ListMessages {
		listAvailableMessages()
		return
	}

	if config.Messages == "" {
		fmt.Printf("Error: --messages flag is required\n")
		fmt.Printf("Example: --messages=\"mavlink:SCALED_IMU2,at:I\"\n")
		os.Exit(1)
	}

	if err := run(config); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() *Config {
	config := &Config{}

	// Main flags
	flag.StringVar(&config.Messages, "messages", "", "Comma-separated list of messages (e.g., mavlink:SCALED_IMU2,at:I)")
	flag.StringVar(&config.OutputFormat, "output", "json", "Output format: json, csv, binary, or multiple (csv,json)")
	flag.StringVar(&config.OutputFile, "file", "cellular_log", "Output file prefix (extension added automatically)")
	flag.IntVar(&config.BufferSize, "buffer", 100, "Log buffer size for batching")
	flag.DurationVar(&config.Interval, "interval", 1*time.Second, "Polling interval")

	// MAVLink flags
	flag.StringVar(&config.MAVDevice, "mav-device", "/dev/ttyUSB0", "MAVLink serial device")
	flag.IntVar(&config.MAVBaud, "mav-baud", 57600, "MAVLink baud rate")
	flag.DurationVar(&config.MAVTimeout, "mav-timeout", 5*time.Second, "MAVLink request timeout")

	// AT flags
	flag.StringVar(&config.ATDevice, "at-device", "/dev/ttyUSB1", "AT command serial device")
	flag.IntVar(&config.ATBaud, "at-baud", 115200, "AT command baud rate")
	flag.DurationVar(&config.ATTimeout, "at-timeout", 5*time.Second, "AT command timeout")

	// Utility flags
	flag.BoolVar(&config.ListMessages, "list", false, "List available messages and exit")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")

	flag.Parse()

	return config
}

func listAvailableMessages() {
	fmt.Println("Available Messages:")
	fmt.Println("\nMAVLink Messages:")
	for name := range mavlinkRegistry {
		fmt.Printf("  mavlink:%s\n", name)
	}

	fmt.Println("\nAT Commands:")
	fmt.Println("  at:I")
	fmt.Println("  at:+GCAP")
	fmt.Println("  at:+CNMI=?")
	fmt.Println("  at:+CREG?")
	fmt.Println("  at:+CSQ")
	fmt.Println("  at:+CPIN?")
	fmt.Println("  (Any valid AT command)")

	fmt.Println("\nExample usage:")
	fmt.Println("  ./logger --messages=\"mavlink:SCALED_IMU2,mavlink:ATTITUDE,at:I,at:+CSQ\"")
}

func run(config *Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messages, err := parseMessages(config.Messages, ctx)
	if err != nil {
		return fmt.Errorf("failed to parse messages: %w", err)
	}

	if len(messages) == 0 {
		return fmt.Errorf("no valid messages specified")
	}

	writer, err := createWriter(config)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}

	processor := cellularlog.NewProcessor(
		ctx,
		config.Interval,
		writer,
		config.BufferSize,
		messages...,
	)

	if err := initializeRequesters(processor, config); err != nil {
		return fmt.Errorf("failed to initialize requesters: %w", err)
	}

	processor.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if config.Verbose {
		fmt.Printf("Starting logger with %d messages, interval: %v\n", len(messages), config.Interval)
		fmt.Printf("Output: %s -> %s\n", config.OutputFormat, config.OutputFile)
	}

	<-sigChan

	if config.Verbose {
		fmt.Println("\nShutting down...")
	}

	// Graceful shutdown
	return processor.Close()
}

func parseMessages(messageStr string, ctx context.Context) ([]cellularlog.Message, error) {
	if messageStr == "" {
		return nil, fmt.Errorf("empty message string")
	}

	parts := strings.Split(messageStr, ",")
	messages := make([]cellularlog.Message, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		colonIndex := strings.Index(part, ":")
		if colonIndex == -1 {
			return nil, fmt.Errorf("invalid message format: %s (expected type:name)", part)
		}

		messageType := strings.TrimSpace(part[:colonIndex])
		messageName := strings.TrimSpace(part[colonIndex+1:])

		switch messageType {
		case "mavlink":
			msg, err := createMAVLinkMessage(messageName, ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to create MAVLink message %s: %w", messageName, err)
			}
			messages = append(messages, msg)

		case "at":
			msg := AT.NewMessage(messageName)
			messages = append(messages, msg)

		default:
			return nil, fmt.Errorf("unknown message type: %s (supported: mavlink, at)", messageType)
		}
	}

	return messages, nil
}

func createMAVLinkMessage(name string, ctx context.Context) (cellularlog.Message, error) {
	if factory, exists := mavlinkRegistry[name]; exists {
		return factory(ctx), nil
	}

	if id, err := strconv.Atoi(name); err == nil {
		return mavlink.CreateMAVLinkMessageByID(uint32(id), ctx)
	}

	return nil, fmt.Errorf("unknown MAVLink message: %s", name)
}

func createWriter(config *Config) (cellularlog.Writer, error) {
	formats := strings.Split(config.OutputFormat, ",")

	if len(formats) == 1 {
		return createSingleWriter(formats[0], config.OutputFile)
	}

	writers := make([]cellularlog.Writer, 0, len(formats))
	for _, format := range formats {
		format = strings.TrimSpace(format)
		writer, err := createSingleWriter(format, config.OutputFile)
		if err != nil {
			for _, w := range writers {
				if err := w.Close(); err != nil {
					fmt.Printf("error closing writer: %v\n", err)
				}
			}
			return nil, err
		}
		writers = append(writers, writer)
	}

	return cellularlog.NewMultiWriter(writers...), nil
}

func createSingleWriter(format, filePrefix string) (cellularlog.Writer, error) {
	switch format {
	case "json":
		filename := filePrefix + ".json"
		return cellularlog.NewJSONWriter(filename)
	case "csv":
		filename := filePrefix + ".csv"
		return cellularlog.NewCSVWriter(filename)
	case "binary":
		filename := filePrefix + ".bin"
		return cellularlog.NewBinaryWriter(filename)
	default:
		return nil, fmt.Errorf("unsupported output format: %s", format)
	}
}

func initializeRequesters(processor *cellularlog.Processor, config *Config) error {
	if needsMAVLink(config.Messages) {
		if config.Verbose {
			fmt.Printf("Initializing MAVLink on %s at %d baud\n", config.MAVDevice, config.MAVBaud)
		}

		mav, err := mavlink.NewMavlink(
			config.MAVDevice,
			config.MAVBaud,
			config.MAVTimeout,
			all.Dialect,
			gomavlib.V2,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize MAVLink: %w", err)
		}

		processor.Mavlink = mav
	}

	// Check if we need AT
	if needsAT(config.Messages) {
		if config.Verbose {
			fmt.Printf("Initializing AT commands on %s at %d baud\n", config.ATDevice, config.ATBaud)
		}

		at, err := AT.NewAT(config.ATDevice, config.ATBaud, config.ATTimeout)
		if err != nil {
			return fmt.Errorf("failed to initialize AT: %w", err)
		}

		processor.AT = at
	}

	return nil
}

func needsMAVLink(messages string) bool {
	return strings.Contains(messages, "mavlink:")
}

func needsAT(messages string) bool {
	return strings.Contains(messages, "at:")
}
