# Cellular Localization Logging

A Go-based logging tool for collecting data from MAVLink-enabled devices and cellular modems via AT commands. This tool captures telemetry data and cellular information for analysis and research purposes.

## Features

- **MAVLink Support**: Request and log messages from MAVLink-compatible devices
- **AT Command Interface**: Send AT commands to cellular modems and log responses
- **Multiple Output Formats**: JSON, CSV, and binary output formats
- **Configurable Logging**: Adjustable polling intervals and buffer sizes

## Installation

### Prerequisites
- Go 1.24 or later
- Serial devices accessible (typically `/dev/ttyUSB*` on Linux)
- Appropriate permissions to access serial ports

### Building from Source
```bash
git clone https://github.com/harshabose/cellular_localisation_logging
cd cellular_localisation_logging
go build -o cellular_logger main.go
```

## Usage

### Basic Usage
```bash
./cellular_logger --messages="mavlink:ATTITUDE,at:+CSQ" --output=json
```

### Command Line Options

| Flag            | Description                                   | Default      |
|-----------------|-----------------------------------------------|--------------|
| `--messages`    | Comma-separated list of messages to log       | Required     |
| `--output`      | Output format: json, csv, binary, or multiple | json         |
| `--file`        | Output file prefix                            | cellular_log |
| `--interval`    | Polling interval                              | 1s           |
| `--buffer`      | Log buffer size for batching                  | 100          |
| `--mav-device`  | MAVLink serial device                         | /dev/ttyUSB0 |
| `--mav-baud`    | MAVLink baud rate                             | 57600        |
| `--mav-timeout` | MAVLink request timeout                       | 5s           |
| `--at-device`   | AT command serial device                      | /dev/ttyUSB1 |
| `--at-baud`     | AT command baud rate                          | 115200       |
| `--at-timeout`  | AT command timeout                            | 5s           |
| `--list`        | List available messages and exit              | false        |

### Message Types

#### MAVLink Messages

##### IMU and Sensor Data:
- `mavlink:SCALED_IMU` - Scaled IMU data (primary)
- `mavlink:SCALED_IMU2` - Scaled IMU data (secondary)
- `mavlink:SCALED_IMU3` - Scaled IMU data (tertiary)
- `mavlink:RAW_IMU` - Raw IMU measurements
- `mavlink:SCALED_PRESSURE` - Barometric pressure/altitude

##### GPS and Positioning:
- `mavlink:GPS_RAW_INT` - Raw GPS data
- `mavlink:GPS2_RAW` - Secondary GPS data
- `mavlink:GPS_STATUS` - GPS satellite count and accuracy metrics
- `mavlink:GLOBAL_POSITION_INT` - Fused global position
- `mavlink:LOCAL_POSITION_NED` - Local position in NED frame

##### Attitude and Orientation:
- `mavlink:ATTITUDE` - Vehicle attitude (Euler angles)
- `mavlink:ATTITUDE_QUATERNION` - Vehicle attitude (quaternions)

##### Navigation and Control:
- `mavlink:NAV_CONTROLLER_OUTPUT` - Navigation controller data
- `mavlink:POSITION_TARGET_GLOBAL_INT` - Global position targets
- `mavlink:POSITION_TARGET_LOCAL_NED` - Local position targets

##### System and Filter Status:
- `mavlink:SYS_STATUS` - System status and health
- `mavlink:HEARTBEAT` - System heartbeat
- `mavlink:EKF_STATUS_REPORT` - Extended Kalman Filter status
- `mavlink:AHRS` - Attitude and Heading Reference System
- `mavlink:AHRS2` - Secondary AHRS data

##### Visual and Optical:
- `mavlink:OPTICAL_FLOW` - Optical flow data
- `mavlink:OPTICAL_FLOW_RAD` - Optical flow (radians)

##### High-Level Data:
- `mavlink:HIGH_LATENCY` - High-latency telemetry
- `mavlink:HIGH_LATENCY2` - Extended high-latency telemetry

##### Using Message IDs:
Any MAVLink message can also be specified using its numeric ID instead of name:
- `mavlink:0` - HEARTBEAT
- `mavlink:24` - GPS_RAW_INT
- `mavlink:30` - ATTITUDE
- `mavlink:33` - GLOBAL_POSITION_INT
- `mavlink:242` - EKF_STATUS_REPORT
- etc.

**Example using IDs:**
```bash
./cellular_logger --messages="mavlink:24,mavlink:30,mavlink:33" --output=json
```
Use ./cellular_logger --list to see all currently supported message names in your installation.





#### AT Commands

##### Basic Modem Information:
- `at:I` - Modem identification
- `at:+GCAP` - Capability list
- 1`at:+CPIN?` - PIN status
##### Network and Signal Quality:
- `at:+CSQ` - Signal quality
- `at:+CREG?` - 2G/3G network registration status
- `at:+CEREG?` - 4G/5G network registration and location info
- `at:+COPS?` - Operator selection and network info
##### Location and Cell Information:
- `at:+CGPADDR` - IP address and location data
- `at:+QENG="servingcell"` - Serving cell information (Quectel modems)
- `at:+QGPSLOC?` - Built-in GPS location (if available)
##### Configuration and Status:
- `at:+CNMI=?` - New message indication settings
- Any valid AT command

### Examples

**Log multiple MAVLink messages:**
```bash
./cellular_logger --messages="mavlink:ATTITUDE,mavlink:GPS_RAW_INT" --interval=500ms
```

**Log cellular data only:**
```bash
./cellular_logger --messages="at:+CSQ,at:+CREG?" --output=csv --interval=2s
```

**Multiple output formats:**
```bash
./cellular_logger --messages="mavlink:ATTITUDE,at:+CSQ" --output=json,csv
```

**List available messages:**
```bash
./cellular_logger --list
```

## Output Formats

### JSON
Each log entry is written as a separate JSON object:
```json
{
  "index": 1,
  "message_type": "mavlink-30",
  "success": true,
  "data": {
    "roll": 0.123,
    "pitch": -0.045,
    "yaw": 1.234
  },
  "request_time": "2024-01-01T12:00:00Z",
  "response_time": "2024-01-01T12:00:00.1Z",
  "duration": 100000000
}
```

### CSV
Tabular format with headers for easy analysis in spreadsheet applications.

### Binary
Compact binary format with length-prefixed JSON entries.

## Hardware Requirements

- **MAVLink Device**: Any device supporting MAVLink protocol (Pixhawk, ArduPilot, etc.)
- **Cellular Modem**: Any modem supporting standard AT commands
- **Serial Connections**: USB-to-serial adapters or direct UART connections

## Troubleshooting

### Permission Issues
```bash
sudo usermod -a -G dialout $USER  # Add user to dialout group
sudo chmod 666 /dev/ttyUSB*       # Or set permissions directly
```

### Device Not Found
- Check device paths with `ls /dev/ttyUSB*`
- Verify devices are connected and recognized
- Use `dmesg` to check for connection messages

### Timeout Errors
- Increase timeout values for slow devices
- Check baud rate settings match your hardware
- Verify cable connections and signal quality

## Dependencies

- [gomavlib](https://github.com/bluenviron/gomavlib) - MAVLink protocol implementation
- [modem](https://github.com/warthog618/modem) - AT command interface
- [gods](https://github.com/emirpasic/gods) - Data structures
- For a complete list of all MAVLink message IDs and definitions, refer to:

  - [MAVLink Common Messages](https://mavlink.io/en/messages/common.html)
  - [ArduPilot-Specific Messages](https://mavlink.io/en/messages/ardupilotmega.html)
  - [All Message Definitions](https://github.com/mavlink/mavlink/tree/master/message_definitions/v1.0)
