import {reactive, toRefs} from 'vue'
import {ListTags, CreateTag, UpdateTag, DeleteTag} from '../../wailsjs/go/main/App'
import type {store} from '../../wailsjs/go/models'

const state = reactive({
  tags: [] as store.Tag[],
  error: '',
})

/**
 * Exported on its own so useConnections can call it after every
 * connection refresh: saving a connection can create Tags, and the
 * filter and the form both need to see them.
 */
export async function refreshTags() {
  try {
    state.tags = await ListTags()
    state.error = ''
  } catch (e) {
    state.error = String(e)
  }
}

/**
 * Neither call refreshes connections itself, which would make the two
 * composables import each other: callers follow up with
 * useConnections().refresh(), which reloads the Tags too.
 */
async function create(tag: store.Tag): Promise<number> {
  try {
    const id = await CreateTag(tag)
    state.error = ''
    return id
  } catch (e) {
    state.error = String(e)
    throw e
  }
}

async function update(tag: store.Tag): Promise<void> {
  try {
    await UpdateTag(tag)
    state.error = ''
  } catch (e) {
    state.error = String(e)
    throw e
  }
}

async function remove(id: number): Promise<void> {
  try {
    await DeleteTag(id)
    state.error = ''
  } catch (e) {
    state.error = String(e)
    throw e
  }
}

export function useTags() {
  return {
    ...toRefs(state),
    refresh: refreshTags,
    create,
    update,
    remove,
  }
}
