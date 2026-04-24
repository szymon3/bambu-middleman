# mqttdump

`mqttdump` is a diagnostic tool that subscribes to the printer's MQTT report topic and writes every message to a JSONL file for offline inspection. It is useful for debugging printer connectivity, understanding print state transitions, and capturing raw MQTT payloads.

`mqttdump` is a developer tool and is not included in release assets. Build it from source:

```bash
go build -o mqttdump ./cmd/mqttdump
```

## Configuration

`mqttdump` uses the same printer connection variables as the observer:

| Variable | Required | Description |
|----------|----------|-------------|
| `PRINTER_IP` | yes | Printer local IP address |
| `PRINTER_SERIAL` | yes | Printer serial number |
| `PRINTER_ACCESS_CODE` | yes | 8-digit access code |
| `DUMP_FILE` | no | Output path; defaults to `mqtt_dump_YYYYMMDD_HHMMSS.jsonl` |

## Usage

```bash
export PRINTER_IP=192.168.1.100
export PRINTER_SERIAL=YOUR_SERIAL
export PRINTER_ACCESS_CODE=12345678
./mqttdump
```

Press Ctrl+C to stop. The tool prints a count of captured messages on exit.

## Output format

Each line is a JSON object:

```json
{"ts":"2026-04-21T15:38:17.123456789Z","payload":{"print":{"gcode_state":"RUNNING","layer_num":42,...}}}
```

- `ts` -- UTC timestamp when the message was received
- `payload` -- the raw MQTT message payload (parsed as JSON, embedded unescaped)
