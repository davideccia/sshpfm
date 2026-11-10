<script lang="ts" setup>
import {computed, nextTick, ref, useId, watch} from 'vue'
import {PhPlus as Plus} from '@phosphor-icons/vue'
import TagChip from './ui/TagChip.vue'
import {MAX_TAG_LABEL, nextSwatch, sameLabel, type TagRef} from '../lib/tags'
import type {store} from '../../wailsjs/go/models'

/**
 * Picks the Tags of a connection. Existing Tags come from `available`; a
 * label that matches none becomes a pending Tag, without an id, which the
 * backend creates only when the connection is saved — cancelling the form
 * leaves no Tag behind.
 */
const props = defineProps<{
  modelValue: TagRef[]
  available: store.Tag[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: TagRef[]]
}>()

type Option =
  | {kind: 'existing', tag: store.Tag}
  | {kind: 'create', label: string}

const query = ref('')
const open = ref(false)
const highlighted = ref(0)
const input = ref<HTMLInputElement | null>(null)

const baseId = useId()
const listboxId = `${baseId}-listbox`
const optionId = (i: number) => `${baseId}-option-${i}`

const chosen = (label: string) => props.modelValue.some(t => sameLabel(t.label, label))

const options = computed<Option[]>(() => {
  const q = query.value.trim()
  const needle = q.toLocaleLowerCase()
  const matches: Option[] = props.available
    .filter(t => !chosen(t.label) && t.label.toLocaleLowerCase().includes(needle))
    .map(tag => ({kind: 'existing', tag}))

  // Offer to create only a label that is neither picked already nor an
  // existing Tag — the latter shows up as its own option above.
  const exists = props.available.some(t => sameLabel(t.label, q))
  if (q && !exists && !chosen(q)) matches.push({kind: 'create', label: q})
  return matches
})

watch(options, () => {
  highlighted.value = Math.min(highlighted.value, Math.max(options.value.length - 1, 0))
})

function add(tag: TagRef) {
  emit('update:modelValue', [...props.modelValue, tag])
  query.value = ''
  highlighted.value = 0
}

function pick(option: Option) {
  if (option.kind === 'existing') {
    add({id: option.tag.id, label: option.tag.label, color: option.tag.color})
  } else {
    const pending = props.modelValue.filter(t => t.id === undefined).length
    add({label: option.label, color: nextSwatch(props.available.length + pending)})
  }
  nextTick(() => input.value?.focus())
}

function removeAt(index: number) {
  const next = [...props.modelValue]
  next.splice(index, 1)
  emit('update:modelValue', next)
}

function onKeydown(e: KeyboardEvent) {
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      open.value = true
      if (options.value.length) highlighted.value = (highlighted.value + 1) % options.value.length
      break
    case 'ArrowUp':
      e.preventDefault()
      open.value = true
      if (options.value.length) {
        highlighted.value = (highlighted.value - 1 + options.value.length) % options.value.length
      }
      break
    case 'Enter':
      // Always swallowed: Enter here means "add this tag", never "submit
      // the whole connection form".
      e.preventDefault()
      if (open.value && options.value[highlighted.value]) pick(options.value[highlighted.value])
      break
    case 'Escape':
      if (open.value) {
        // Close the list without also cancelling whatever holds the form.
        e.stopPropagation()
        open.value = false
      }
      break
    case 'Backspace':
      if (query.value === '' && props.modelValue.length) removeAt(props.modelValue.length - 1)
      break
  }
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <span :id="`${baseId}-label`" class="text-xs font-semibold text-ink-muted">Tags</span>

    <div class="relative">
      <div
        class="flex min-h-[38px] w-full flex-wrap items-center gap-1.5 rounded-xl bg-canvas
               px-2 py-1.5 shadow-neu-in focus-within:outline-2
               focus-within:outline-offset-2 focus-within:outline-accent"
        @click="input?.focus()"
      >
        <TagChip
          v-for="(tag, i) in modelValue"
          :key="tag.id ?? `new-${tag.label}`"
          :label="tag.label"
          :color="tag.color"
          removable
          @remove="removeAt(i)"
        />
        <input
          ref="input"
          v-model="query"
          type="text"
          role="combobox"
          class="min-w-[8ch] flex-1 bg-transparent px-1 py-0.5 text-sm text-ink
                 outline-none placeholder:text-ink-faint"
          :maxlength="MAX_TAG_LABEL"
          :placeholder="modelValue.length ? '' : 'Add a tag…'"
          :aria-labelledby="`${baseId}-label`"
          aria-autocomplete="list"
          :aria-expanded="open && options.length > 0"
          :aria-controls="listboxId"
          :aria-activedescendant="open && options.length ? optionId(highlighted) : undefined"
          @focus="open = true"
          @blur="open = false"
          @input="open = true"
          @keydown="onKeydown"
        />
      </div>

      <!-- Options take mousedown.prevent so clicking one does not blur the
           input and close the list before the click lands. -->
      <ul
        v-show="open && options.length > 0"
        :id="listboxId"
        role="listbox"
        class="absolute inset-x-0 top-full mt-2 flex max-h-56 list-none flex-col
               gap-0.5 overflow-y-auto rounded-xl bg-canvas p-1.5 shadow-neu-out"
      >
        <li
          v-for="(option, i) in options"
          :id="optionId(i)"
          :key="option.kind === 'existing' ? option.tag.id : 'create'"
          role="option"
          :aria-selected="i === highlighted"
          class="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-ink"
          :class="i === highlighted ? 'bg-hover' : ''"
          @mousedown.prevent="pick(option)"
          @mousemove="highlighted = i"
        >
          <template v-if="option.kind === 'existing'">
            <span class="size-2 shrink-0 rounded-full" :style="{backgroundColor: option.tag.color}" aria-hidden="true"/>
            <span class="truncate">{{ option.tag.label }}</span>
          </template>
          <template v-else>
            <Plus :size="12" weight="bold" class="shrink-0 text-accent-ink"/>
            <span class="truncate">Create “{{ option.label }}”</span>
          </template>
        </li>
      </ul>
    </div>
  </div>
</template>
