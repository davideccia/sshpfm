<script lang="ts" setup>
import {computed, nextTick, ref} from 'vue'
import {
  PhArrowLeft as ArrowLeft,
  PhPlus as Plus,
  PhPencilSimple as PencilSimple,
  PhTrash as Trash,
  PhCheck as Check,
  PhWarningCircle as WarningCircle,
} from '@phosphor-icons/vue'
import Button from './ui/Button.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import {useTags} from '../composables/useTags'
import {MAX_TAG_LABEL, TAG_SWATCHES, nextSwatch, sameLabel} from '../lib/tags'
import {store} from '../../wailsjs/go/models'

const emit = defineEmits<{
  'tags-changed': []
  close: []
}>()

// Tag maintenance. Allowed while tunnels are up: a Tag is not part of any
// tunnel's settings, unlike the connection fields the form edits.
const {tags, create: createTag, update: updateTag, remove: removeTag} = useTags()

// id 0 is a Tag being added: it is not in the store yet, so it rides at
// the top of the list as a row of its own until it is saved or cancelled.
const NEW_TAG = 0

const editing = ref<{id: number, label: string, color: string} | null>(null)
const tagError = ref('')
const deleting = ref<store.Tag | null>(null)

const rows = computed<store.Tag[]>(() =>
  editing.value?.id === NEW_TAG
    ? [store.Tag.createFrom({id: NEW_TAG, label: '', color: editing.value.color}), ...tags.value]
    : tags.value
)

const labelInput = ref<HTMLInputElement | null>(null)

// Opening the editor puts the caret in the label, so add and rename are
// both one click away from typing.
function focusLabel() {
  nextTick(() => labelInput.value?.focus())
}

function startAddTag() {
  tagError.value = ''
  editing.value = {id: NEW_TAG, label: '', color: nextSwatch(tags.value.length)}
  focusLabel()
}

function startEditTag(tag: store.Tag) {
  tagError.value = ''
  editing.value = {id: tag.id, label: tag.label, color: tag.color}
  focusLabel()
}

function cancelEditTag() {
  editing.value = null
  tagError.value = ''
}

async function saveTag() {
  if (!editing.value) return
  const label = editing.value.label.trim()
  if (!label) {
    tagError.value = 'Label is required'
    return
  }
  const id = editing.value.id
  if (tags.value.some(t => t.id !== id && sameLabel(t.label, label))) {
    tagError.value = `A tag named “${label}” already exists`
    return
  }
  try {
    const tag = store.Tag.createFrom({id, label, color: editing.value.color})
    if (id === NEW_TAG) await createTag(tag)
    else await updateTag(tag)
    editing.value = null
    tagError.value = ''
    emit('tags-changed')
  } catch (e) {
    tagError.value = String(e)
  }
}

function deleteMessage(tag: store.Tag): string {
  const n = tag.connectionCount ?? 0
  const usage = n === 0
    ? 'No connection uses it.'
    : n === 1
      ? 'It will be removed from 1 connection; the connection stays.'
      : `It will be removed from ${n} connections; the connections stay.`
  return `Delete tag “${tag.label}”? ${usage}`
}

async function confirmDeleteTag() {
  const tag = deleting.value
  deleting.value = null
  if (!tag) return
  try {
    await removeTag(tag.id)
    if (editing.value?.id === tag.id) editing.value = null
    emit('tags-changed')
  } catch (e) {
    tagError.value = String(e)
  }
}
</script>

<template>
  <section class="flex-1 overflow-y-auto bg-canvas p-8">
    <div class="flex max-w-[560px] flex-col gap-4">
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="inline-flex cursor-pointer items-center justify-center
                 rounded-lg p-1.5 text-ink-muted transition-colors duration-150
                 outline-none hover:bg-hover hover:text-accent-ink
                 focus-visible:outline-2 focus-visible:outline-offset-1
                 focus-visible:outline-accent"
          aria-label="Back"
          @click="emit('close')"
        >
          <ArrowLeft :size="18"/>
        </button>
        <h2 class="m-0 flex-1 text-xl font-semibold text-ink">Tags</h2>
        <Button variant="primary" :disabled="editing?.id === NEW_TAG" @click="startAddTag">
          <Plus :size="16" weight="bold"/>
          Add tag
        </Button>
      </div>

      <div class="flex flex-col gap-3 rounded-2xl bg-canvas p-4 shadow-neu-out">
        <p class="m-0 text-sm leading-relaxed text-ink-muted">
          Add, rename, recolour or delete tags. A connection's form can also
          create one as you type it.
        </p>

        <p v-if="rows.length === 0" class="m-0 text-sm text-ink-faint">No tags yet.</p>

        <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
          <li v-for="tag in rows" :key="tag.id">
            <form
              v-if="editing?.id === tag.id"
              class="flex flex-col gap-3 rounded-xl p-3 shadow-neu-in-sm"
              @submit.prevent="saveTag"
              @keydown.esc.prevent="cancelEditTag"
            >
              <label class="flex flex-col gap-1">
                <span class="text-xs font-semibold text-ink-muted">Label</span>
                <input
                  :ref="el => { labelInput = el as HTMLInputElement | null }"
                  v-model="editing.label"
                  type="text"
                  :maxlength="MAX_TAG_LABEL"
                  class="w-full rounded-xl bg-canvas px-3 py-2 text-sm text-ink shadow-neu-in
                         outline-none focus-visible:outline-2 focus-visible:outline-offset-2
                         focus-visible:outline-accent"
                  :class="tagError ? 'border border-danger' : ''"
                  :aria-invalid="tagError ? 'true' : undefined"
                />
              </label>

              <fieldset class="m-0 flex flex-col gap-1.5 border-0 p-0">
                <legend class="mb-1 p-0 text-xs font-semibold text-ink-muted">Colour</legend>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    v-for="swatch in TAG_SWATCHES"
                    :key="swatch"
                    type="button"
                    class="inline-flex size-7 cursor-pointer items-center justify-center rounded-lg
                           bg-canvas outline-none transition-[box-shadow] duration-150
                           focus-visible:outline-2 focus-visible:outline-offset-1
                           focus-visible:outline-accent"
                    :class="editing.color === swatch ? 'shadow-neu-in-sm' : 'shadow-neu-out-sm'"
                    :aria-label="`Colour ${swatch}`"
                    :aria-pressed="editing.color === swatch"
                    @click="editing.color = swatch"
                  >
                    <span class="size-3 rounded-full" :style="{backgroundColor: swatch}"/>
                  </button>
                  <!-- Native picker for any other colour: on macOS it opens
                       the system colour panel. -->
                  <label
                    class="inline-flex h-7 cursor-pointer items-center gap-2 rounded-lg px-2
                           text-xs text-ink-muted shadow-neu-out-sm
                           focus-within:outline-2 focus-within:outline-offset-1
                           focus-within:outline-accent"
                  >
                    <input
                      v-model="editing.color"
                      type="color"
                      class="size-4 cursor-pointer rounded-full border-0 bg-transparent p-0"
                    />
                    Custom
                  </label>
                  <code class="font-mono text-xs text-ink-faint">{{ editing.color }}</code>
                </div>
              </fieldset>

              <p v-if="tagError" role="alert" class="m-0 flex items-start gap-2 text-xs text-danger">
                <WarningCircle :size="14" weight="fill" class="mt-px shrink-0"/>
                {{ tagError }}
              </p>

              <div class="flex justify-end gap-2">
                <Button variant="secondary" @click="cancelEditTag">Cancel</Button>
                <Button variant="primary" type="submit">
                  <Check :size="16" weight="bold"/>
                  {{ editing.id === NEW_TAG ? 'Add' : 'Save' }}
                </Button>
              </div>
            </form>

            <div v-else class="flex items-center gap-3 rounded-xl px-3 py-2">
              <span class="size-2.5 shrink-0 rounded-full" :style="{backgroundColor: tag.color}" aria-hidden="true"/>
              <span class="min-w-0 flex-1 truncate text-sm text-ink">{{ tag.label }}</span>
              <span class="shrink-0 text-xs text-ink-faint">
                {{ tag.connectionCount ?? 0 }} connection{{ (tag.connectionCount ?? 0) === 1 ? '' : 's' }}
              </span>
              <button
                type="button"
                class="inline-flex cursor-pointer items-center justify-center rounded-lg p-1.5
                       text-ink-muted transition-colors duration-150 outline-none
                       hover:bg-hover hover:text-accent-ink focus-visible:outline-2
                       focus-visible:outline-offset-1 focus-visible:outline-accent"
                :aria-label="`Edit tag ${tag.label}`"
                @click="startEditTag(tag)"
              >
                <PencilSimple :size="15"/>
              </button>
              <button
                type="button"
                class="inline-flex cursor-pointer items-center justify-center rounded-lg p-1.5
                       text-ink-muted transition-colors duration-150 outline-none
                       hover:bg-danger-wash hover:text-danger focus-visible:outline-2
                       focus-visible:outline-offset-1 focus-visible:outline-accent"
                :aria-label="`Delete tag ${tag.label}`"
                @click="deleting = tag"
              >
                <Trash :size="15"/>
              </button>
            </div>
          </li>
        </ul>

        <p v-if="tagError && !editing" role="alert" class="m-0 flex items-start gap-2 text-xs text-danger">
          <WarningCircle :size="14" weight="fill" class="mt-px shrink-0"/>
          {{ tagError }}
        </p>
      </div>
    </div>

    <ConfirmDialog
      v-if="deleting"
      title="Delete tag"
      :message="deleteMessage(deleting)"
      @confirm="confirmDeleteTag"
      @cancel="deleting = null"
    />
  </section>
</template>
