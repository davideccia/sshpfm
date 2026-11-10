import {reactive, toRefs} from 'vue'
import {CreateConnection, ListConnections, UpdateConnection, DeleteConnection} from '../../wailsjs/go/main/App'
import {store} from '../../wailsjs/go/models'
import {refreshTags} from './useTags'

/**
 * What the form produces. `id` is assigned by the backend, and
 * `hasPassword` and `tags` are read-only reports from the store — the form
 * sends the password itself in `password`, or null to leave the stored one
 * alone, and the full Tag set in `tagIds` plus `newTags`.
 */
export type ConnectionDraft = Omit<store.Connection, 'id' | 'hasPassword' | 'tags' | 'convertValues'>

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
    // Saving a connection can create Tags, and a Tag rename shows up on
    // every chip, so the two lists are always reloaded together.
    await refreshTags()
  } catch (e) {
    state.error = String(e)
  } finally {
    state.isLoading = false
  }
}

async function create(connection: ConnectionDraft): Promise<number> {
  try {
    const id = await CreateConnection(store.Connection.createFrom({id: 0, hasPassword: false, tags: [], ...connection}))
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
