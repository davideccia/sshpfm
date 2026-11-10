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

export function useTags() {
  return {
    ...toRefs(state),
    refresh: refreshTags,
    // The mutations hand their errors straight to the caller, which shows
    // them beside the form. state.error is kept for refreshTags alone, so
    // the sidebar banner does not repeat a message TagsView already shows.
    // None of them refreshes connections itself, which would make the two
    // composables import each other: callers follow up with
    // useConnections().refresh(), which reloads the Tags too.
    create: (tag: store.Tag): Promise<number> => CreateTag(tag),
    update: (tag: store.Tag): Promise<void> => UpdateTag(tag),
    remove: (id: number): Promise<void> => DeleteTag(id),
  }
}
