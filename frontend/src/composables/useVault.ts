import {reactive, toRefs} from 'vue'
import {
  VaultState,
  CreateVault,
  UnlockVault,
  ResetVault,
} from '../../wailsjs/go/main/App'

/**
 * `unset` is a first run with no master password yet, `locked` a vault
 * waiting to be opened, and `unlocked` the only state in which the rest
 * of the app renders. `unknown` is the value before the first check, so
 * the gate does not flash the wrong form on startup.
 */
export type VaultStatus = 'unknown' | 'unset' | 'locked' | 'unlocked'

const state = reactive({
  status: 'unknown' as VaultStatus,
  error: '',
  isBusy: false,
})

async function refresh() {
  try {
    state.status = (await VaultState()) as VaultStatus
    state.error = ''
  } catch (e) {
    state.error = String(e)
  }
}

/**
 * Deriving the key costs about 100ms of deliberate work, so every action
 * flags `isBusy` for the form to disable its button rather than let the
 * user queue a second attempt.
 */
async function run(action: () => Promise<void>): Promise<boolean> {
  state.error = ''
  state.isBusy = true
  try {
    await action()
    await refresh()
    return true
  } catch (e) {
    state.error = String(e)
    return false
  } finally {
    state.isBusy = false
  }
}

async function create(master: string): Promise<boolean> {
  return run(() => CreateVault(master))
}

async function unlock(master: string): Promise<boolean> {
  return run(() => UnlockVault(master))
}

async function reset(): Promise<boolean> {
  return run(() => ResetVault())
}

export function useVault() {
  return {
    ...toRefs(state),
    refresh,
    create,
    unlock,
    reset,
  }
}
