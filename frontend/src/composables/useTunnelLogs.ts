import {reactive} from 'vue'
import {ConnectionLogs} from '../../wailsjs/go/main/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'
import type {tunnel} from '../../wailsjs/go/models'

// Matches app.go's TunnelLogEvent, emitted on the "tunnel:log" event. Not a
// bound method's parameter/return type, so Wails does not generate a class
// for it; typed by hand against its json tags.
interface TunnelLogEvent {
  id: number
  log: tunnel.LogLine
}

const MAX_LINES = 300

const state = reactive({
  // Keyed by connection id. A connection with no entry has never been
  // fetched or streamed to yet.
  logs: {} as Record<number, tunnel.LogLine[]>,
})

let subscribed = false

function push(id: number, line: tunnel.LogLine) {
  const lines = state.logs[id] ?? (state.logs[id] = [])
  lines.push(line)
  if (lines.length > MAX_LINES) lines.splice(0, lines.length - MAX_LINES)
}

function subscribeOnce() {
  if (subscribed) return
  subscribed = true
  EventsOn('tunnel:log', (event: TunnelLogEvent) => {
    push(event.id, event.log)
  })
}

/**
 * Fetches a connection's log history from the backend, replacing whatever
 * is held locally. Used to backfill lines emitted before a log panel was
 * open to receive the live stream.
 */
async function load(id: number): Promise<void> {
  try {
    state.logs[id] = await ConnectionLogs(id)
  } catch {
    // Leave whatever was already streamed in; a failed backfill should not
    // blank out lines the panel already has.
  }
}

function logsOf(id: number | null): tunnel.LogLine[] {
  if (id === null) return []
  return state.logs[id] ?? []
}

export function useTunnelLogs() {
  subscribeOnce()
  return {
    load,
    logsOf,
  }
}
