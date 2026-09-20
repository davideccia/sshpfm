import {reactive, toRefs} from 'vue'
import {
  StartTunnel,
  StopTunnel,
  TunnelStatuses,
  TrustHostKey,
  OpenInBrowser,
} from '../../wailsjs/go/main/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'
import type {tunnel} from '../../wailsjs/go/models'

export type TunnelState = 'stopped' | 'connecting' | 'active' | 'error'

const state = reactive({
  // Keyed by connection id. A connection with no entry has never been started.
  statuses: {} as Record<number, tunnel.Status>,
  error: '',
})

let subscribed = false

/**
 * Starts listening for backend status events and loads the current
 * snapshot. Safe to call from several components; only the first call
 * subscribes.
 */
async function init() {
  // Snapshot first: refresh() clears state.error on success, so a
  // subscription failure reported before it would be wiped out.
  await refresh()
  if (subscribed) return

  try {
    EventsOn('tunnel:status', (status: tunnel.Status) => {
      state.statuses[status.id] = status
    })
    // Flagged only once the subscription is in place, so a failure here
    // does not lock the app out of ever retrying.
    subscribed = true
  } catch (e) {
    // Without the subscription the panel would silently freeze on the
    // snapshot above instead of tracking the tunnel.
    state.error = `Tunnel updates unavailable: ${e}`
  }
}

async function refresh() {
  try {
    const all = await TunnelStatuses()
    const next: Record<number, tunnel.Status> = {}
    for (const s of all) next[s.id] = s
    state.statuses = next
    state.error = ''
  } catch (e) {
    state.error = String(e)
  }
}

function statusOf(id: number): tunnel.Status | null {
  return state.statuses[id] ?? null
}

function stateOf(id: number): TunnelState {
  return (state.statuses[id]?.state as TunnelState) ?? 'stopped'
}

async function start(id: number): Promise<void> {
  state.error = ''
  try {
    await StartTunnel(id)
  } catch (e) {
    // The backend also emits an error status; keep the message here so
    // the detail panel can show it even before the event lands.
    state.error = String(e)
  }
}

async function stop(id: number): Promise<void> {
  state.error = ''
  try {
    await StopTunnel(id)
    // A stopped tunnel is dropped from the backend registry, so the
    // final event may not carry it; drop it locally too.
    delete state.statuses[id]
  } catch (e) {
    state.error = String(e)
  }
}

async function openInBrowser(id: number): Promise<void> {
  state.error = ''
  try {
    await OpenInBrowser(id)
  } catch (e) {
    state.error = String(e)
  }
}

async function trustHostKey(id: number): Promise<void> {
  state.error = ''
  try {
    await TrustHostKey(id)
  } catch (e) {
    state.error = String(e)
  }
}

export function useTunnels() {
  return {
    ...toRefs(state),
    init,
    refresh,
    statusOf,
    stateOf,
    start,
    stop,
    openInBrowser,
    trustHostKey,
  }
}
