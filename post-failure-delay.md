# Deferred: Reliable `layer_num` for FAILED prints via post-print message

## Background

The Bambu P1S reliably sends a second `FAILED` message ~18 seconds after a print ends (failed
or cancelled). This post-print idle message carries the correct `layer_num` (1-indexed). The
first `FAILED` message (the terminal one that triggers the download) does **not** carry
`layer_num` in incremental MQTT updates — only the post-print idle message does.

This means the current implementation (`LastLayerNum` captured only during `RUNNING`/`PREPARE`)
produces `LastLayerNum=0` for most failed prints, falling back to full-file usage with a WARN
log. The only way to get a reliable `layer_num` today is if Home Assistant's `PUSH_ALL` happens
to arrive during the print.

## Proposed fix: delayed FAILED event emission

### State machine changes in `printer/mqtt.go`

Add fields to `MQTTClient`:

```go
pendingFailedEvent *PrintEvent  // non-nil while awaiting post-print layer_num
pendingCancel      chan struct{} // close this to cancel the wait goroutine
```

**On first `FAILED` + `printActive=true`:**
1. Build the `PrintEvent` (file, subtask, state) but **do not emit yet**.
2. Store as `pendingFailedEvent`.
3. Reset `printActive=false`, `lastGCodeFile=""`, `lastSubtaskName=""`, `lastLayerNum=0`.
4. Start a goroutine:
   ```go
   go func() {
       select {
       case <-time.After(30 * time.Second):
           c.mu.Lock()
           defer c.mu.Unlock()
           if c.pendingFailedEvent != nil {
               c.events <- *c.pendingFailedEvent  // emit with LastLayerNum=0 fallback
               c.pendingFailedEvent = nil
           }
       case <-c.pendingCancel:
           // cancelled — event was already emitted with correct layer_num
       }
   }()
   ```

**On subsequent `FAILED` with `pendingFailedEvent != nil` and `layer_num > 0`:**
1. Set `pendingFailedEvent.LastLayerNum = layer_num`.
2. `close(c.pendingCancel)` — cancels the timer goroutine.
3. Emit `*pendingFailedEvent` to the events channel.
4. Set `pendingFailedEvent = nil`.

**`FINISH` is unchanged** — emit immediately, `ComputedUsage(0)` in `loop.go`.

### Why `loop.go` needs no changes

`LastLayerNum` will now be reliably set before the event reaches the observer. The existing
branch in `logResult` already handles both cases:
- `LastLayerNum > 0` → `ComputedUsage(LastLayerNum)`
- `LastLayerNum == 0` → fallback to `ComputedUsage(0)` with WARN (timeout path)

### Safety properties

- **30s timeout** — daemon never hangs; if the post-print message never arrives, the event
  fires with graceful fallback.
- **Duplicate FAILED immunity** — after `pendingFailedEvent` is cleared, subsequent `FAILED`
  messages are ignored (no `printActive`, no `pendingFailedEvent`).
- **Mutex discipline** — the timer goroutine must re-acquire `c.mu` before touching
  `pendingFailedEvent` or the events channel. The cancel path doesn't touch shared state.
- **`FINISH` unaffected** — no delay introduced for successful prints.

### Observed timing

From real print data: post-print `FAILED` with `layer_num` arrived **18 seconds** after the
terminal event. The 30s timeout provides comfortable headroom.
