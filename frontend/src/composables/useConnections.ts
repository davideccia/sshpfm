import {reactive, toRefs} from 'vue'
import {CreateConnection, ListConnections, UpdateConnection, DeleteConnection} from '../../wailsjs/go/main/App'
import type {store} from '../../wailsjs/go/models'

/**
 * What the form produces. `id` is assigned by the backend, and
 * `hasPassword` is a read-only report from the store — the form sends the
 * password itself in `password`, or null to leave the stored one alone.
 */
export type ConnectionDraft = Omit<store.Connection, 'id' | 'hasPassword'>

const state = reactive({
  connections: [] as store.Connection[],
  isLoading: false,
  error: '',
})

async function refresh() {
  state.isLoading = true
  try {
    state.connections = await ListConnections()
    state.error = ''
  } catch (e) {
    state.error = String(e)
  } finally {
    state.isLoading = false
  }
}

async function create(connection: ConnectionDraft): Promise<number> {
  try {
    const id = await CreateConnection({id: 0, hasPassword: false, ...connection} as store.Connection)
    state.error = ''
    await refresh()
    return id
  } catch (e) {
    state.error = String(e)
    throw e
  }
}

async function update(connection: store.Connection): Promise<void> {
  try {
    await UpdateConnection(connection)
    state.error = ''
    await refresh()
  } catch (e) {
    state.error = String(e)
    throw e
  }
}

async function remove(id: number): Promise<void> {
  try {
    await DeleteConnection(id)
    state.error = ''
    await refresh()
  } catch (e) {
    state.error = String(e)
    throw e
  }
}

export function useConnections() {
  return {
    ...toRefs(state),
    refresh,
    create,
    update,
    remove,
  }
}
