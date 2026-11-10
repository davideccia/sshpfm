<script lang="ts" setup>
import {ref, computed, watch} from 'vue'
import {
  PhPlus as Plus,
  PhGearSix as GearSix,
  PhTag as TagIcon,
  PhWarningCircle as WarningCircle,
  PhArrowSquareOut as ArrowSquareOut,
  PhPower as Power,
  PhCircleNotch as CircleNotch,
  PhTrash as Trash,
} from '@phosphor-icons/vue'
import Button from './ui/Button.vue'
import StatusDot from './ui/StatusDot.vue'
import TagChip from './ui/TagChip.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import {useTunnels} from '../composables/useTunnels'
import type {store} from '../../wailsjs/go/models'

const props = defineProps<{
  connections: store.Connection[]
  tags: store.Tag[]
  selectedId: number | null
  settingsActive: boolean
  tagsActive: boolean
  isLoading: boolean
  error: string
}>()

const {stateOf, start, stop, openInBrowser} = useTunnels()

const emit = defineEmits<{
  select: [id: number]
  create: []
  settings: []
  tags: []
  'delete-many': [ids: number[]]
}>()

function toggleTunnel(id: number) {
  stateOf(id) === 'active' ? stop(id) : start(id)
}

// The Tag filter: a Connection is visible only when it carries every
// selected Tag. Held in memory on purpose, so every launch starts from
// the full list rather than one where connections seem to have vanished.
const activeTagIds = ref<Set<number>>(new Set())

function toggleTag(id: number) {
  const next = new Set(activeTagIds.value)
  next.has(id) ? next.delete(id) : next.add(id)
  activeTagIds.value = next
}

function clearTagFilter() {
  activeTagIds.value = new Set()
}

// A Tag deleted in Settings must not keep filtering from beyond the grave.
watch(() => props.tags, (tags) => {
  const known = new Set(tags.map(t => t.id))
  if ([...activeTagIds.value].some(id => !known.has(id))) {
    activeTagIds.value = new Set([...activeTagIds.value].filter(id => known.has(id)))
  }
})

const visibleConnections = computed(() => {
  if (activeTagIds.value.size === 0) return props.connections
  return props.connections.filter(c =>
    [...activeTagIds.value].every(id => c.tags.some(t => t.id === id))
  )
})

// The sidebar row has room for two chips; the rest collapse into "+N".
const ROW_TAGS = 2

const selectionMode = ref(false)
const selectedIds = ref<Set<number>>(new Set())
const confirmingBulkDelete = ref(false)

function startSelection() {
  selectionMode.value = true
  selectedIds.value = new Set()
}

function cancelSelection() {
  selectionMode.value = false
  selectedIds.value = new Set()
}

function toggleSelected(id: number) {
  const next = new Set(selectedIds.value)
  next.has(id) ? next.delete(id) : next.add(id)
  selectedIds.value = next
}

// Selection only ever covers what is on screen: "All" means all visible,
// and a connection the filter hides drops out of the selection, so the
// Delete count never includes something the user cannot see.
watch(visibleConnections, (visible) => {
  const ids = new Set(visible.map(c => c.id))
  if ([...selectedIds.value].some(id => !ids.has(id))) {
    selectedIds.value = new Set([...selectedIds.value].filter(id => ids.has(id)))
  }
})

const allSelected = computed(() =>
  visibleConnections.value.length > 0 && selectedIds.value.size === visibleConnections.value.length
)

function toggleSelectAll() {
  selectedIds.value = allSelected.value
    ? new Set()
    : new Set(visibleConnections.value.map(c => c.id))
}

function confirmBulkDelete() {
  emit('delete-many', [...selectedIds.value])
  confirmingBulkDelete.value = false
  cancelSelection()
}
</script>

<template>
  <aside
    class="flex w-[300px] shrink-0 flex-col gap-4 overflow-y-auto
           bg-canvas px-4 py-5 shadow-neu-panel-edge"
  >
    <!-- Set in the dot-matrix "Dited" face so it reads as the wordmark
         without shipping it as an image. Living here rather than in a
         separate app header keeps the sidebar from reading as a region cut
         off from its own title. Sized to the sidebar's own width, in the
         accent colour so it reads as the brand mark rather than a label.
         The convex shadow stands in for an outline — a real border would
         be the one flat element left in an otherwise neumorphic sidebar. -->
    <div class="rounded-2xl bg-canvas px-4 py-3 shadow-neu-out">
      <span
        class="block whitespace-nowrap text-center font-display text-6xl
               leading-none tracking-tight text-accent-ink select-none"
      >sshpfm</span>
    </div>

    <!-- Filled with the hover grey rather than the canvas the logo card
         sits on, so it reads as a distinct, clickable destination without
         reaching for the accent, which is reserved for primary actions. -->
    <!-- Two destinations side by side, sharing one look so neither reads
         as primary. -->
    <nav class="grid grid-cols-2 gap-3" aria-label="App sections">
      <button
        type="button"
        class="flex w-full cursor-pointer items-center justify-center gap-2
               rounded-2xl bg-hover px-4 py-2.5 text-sm font-semibold text-ink
               transition-[box-shadow] duration-150 outline-none
               focus-visible:outline-2 focus-visible:-outline-offset-2
               focus-visible:outline-accent
               active:scale-[0.99]"
        :class="settingsActive ? 'shadow-neu-in' : 'shadow-neu-out hover:shadow-neu-out-sm active:shadow-neu-in'"
        :aria-current="settingsActive ? 'true' : undefined"
        @click="emit('settings')"
      >
        <GearSix :size="16" weight="bold"/>
        Settings
      </button>
      <button
        type="button"
        class="flex w-full cursor-pointer items-center justify-center gap-2
               rounded-2xl bg-hover px-4 py-2.5 text-sm font-semibold text-ink
               transition-[box-shadow] duration-150 outline-none
               focus-visible:outline-2 focus-visible:-outline-offset-2
               focus-visible:outline-accent
               active:scale-[0.99]"
        :class="tagsActive ? 'shadow-neu-in' : 'shadow-neu-out hover:shadow-neu-out-sm active:shadow-neu-in'"
        :aria-current="tagsActive ? 'true' : undefined"
        @click="emit('tags')"
      >
        <TagIcon :size="16" weight="bold"/>
        Tags
      </button>
    </nav>

    <div class="flex flex-col gap-2">
      <div class="flex items-center justify-between">
        <h2 class="text-base font-semibold text-ink">Connections</h2>

        <div v-if="!selectionMode" class="flex items-center gap-2">
          <button
            v-if="connections.length > 0"
            type="button"
            class="inline-flex cursor-pointer items-center justify-center
                   rounded-xl p-2 text-ink-muted shadow-neu-out
                   transition-[box-shadow,color] duration-150 outline-none
                   hover:text-danger focus-visible:outline-2
                   focus-visible:outline-offset-2 focus-visible:outline-accent
                   active:shadow-neu-in active:scale-[0.99]"
            aria-label="Select connections to delete"
            title="Select connections to delete"
            @click="startSelection"
          >
            <Trash :size="16" weight="bold"/>
          </button>
          <Button variant="primary" @click="emit('create')">
            <Plus :size="16" weight="bold"/>
            New
          </Button>
        </div>
      </div>

      <div v-if="selectionMode" class="flex items-center justify-between gap-2">
        <label
          class="flex cursor-pointer items-center gap-1.5 rounded-xl px-1 py-2
                 text-sm font-semibold text-ink-muted transition-colors
                 duration-150 hover:text-ink"
        >
          <input
            type="checkbox"
            class="size-4 shrink-0 cursor-pointer accent-accent-strong"
            :checked="allSelected"
            @change="toggleSelectAll"
          />
          All
        </label>

        <div class="flex items-center gap-2">
          <button
            type="button"
            class="cursor-pointer rounded-xl px-2 py-2 text-sm font-semibold
                   text-ink-muted transition-colors duration-150 outline-none
                   hover:text-ink focus-visible:outline-2
                   focus-visible:outline-offset-2 focus-visible:outline-accent"
            @click="cancelSelection"
          >
            Cancel
          </button>
          <Button
            variant="danger"
            class="min-w-0"
            :disabled="selectedIds.size === 0"
            @click="confirmingBulkDelete = true"
          >
            <Trash :size="16" weight="bold" class="shrink-0"/>
            <span class="truncate">Delete ({{ selectedIds.size }})</span>
          </Button>
        </div>
      </div>
    </div>

    <!-- p-1 leaves room for the chips' shadows inside the scroll box. -->
    <div v-if="tags.length > 0" class="flex flex-col gap-1.5">
      <div class="flex items-center justify-between">
        <span id="tag-filter-label" class="text-xs font-semibold text-ink-muted">Filter by tag</span>
        <button
          v-if="activeTagIds.size > 0"
          type="button"
          class="cursor-pointer rounded-md text-xs text-ink-faint underline-offset-2
                 transition-colors duration-150 outline-none hover:text-ink-muted
                 hover:underline focus-visible:outline-2
                 focus-visible:outline-offset-2 focus-visible:outline-accent"
          @click="clearTagFilter"
        >
          Clear
        </button>
      </div>
      <div
        role="group"
        aria-labelledby="tag-filter-label"
        class="-m-1 flex max-h-24 flex-wrap gap-1.5 overflow-y-auto p-1"
      >
        <button
          v-for="tag in tags"
          :key="tag.id"
          type="button"
          class="inline-flex max-w-full min-w-0 cursor-pointer items-center gap-1.5
                 rounded-lg bg-canvas px-2 py-0.5 text-xs transition-[box-shadow,color]
                 duration-150 outline-none focus-visible:outline-2
                 focus-visible:outline-offset-1 focus-visible:outline-accent"
          :class="activeTagIds.has(tag.id)
            ? 'text-ink shadow-neu-in-sm'
            : 'text-ink-muted shadow-neu-out-sm hover:text-ink'"
          :aria-pressed="activeTagIds.has(tag.id)"
          @click="toggleTag(tag.id)"
        >
          <span class="size-2 shrink-0 rounded-full" :style="{backgroundColor: tag.color}" aria-hidden="true"/>
          <span class="truncate">{{ tag.label }}</span>
        </button>
      </div>
    </div>

    <p
      v-if="error"
      role="alert"
      class="flex items-start gap-2 rounded-lg border border-danger/40
             bg-danger-wash px-3 py-2 text-xs leading-relaxed text-danger"
    >
      <WarningCircle :size="16" weight="fill" class="mt-px shrink-0"/>
      {{ error }}
    </p>

    <p v-if="isLoading" class="text-sm text-ink-muted">Loading…</p>

    <p v-else-if="connections.length === 0" class="text-sm leading-relaxed text-ink-muted">
      No connections yet.<br/>
      Create one to get started.
    </p>

    <div v-else-if="visibleConnections.length === 0" class="flex flex-col items-start gap-2">
      <p class="m-0 text-sm leading-relaxed text-ink-muted">No connections with all of these tags.</p>
      <Button variant="secondary" @click="clearTagFilter">Clear filters</Button>
    </div>

    <ul v-else class="flex list-none flex-col gap-1">
      <li v-for="c in visibleConnections" :key="c.id" class="relative">
        <button
          type="button"
          class="relative w-full cursor-pointer rounded-xl px-3 py-2.5 text-left
                 transition-[box-shadow] duration-150 outline-none
                 hover:shadow-neu-out-sm
                 focus-visible:outline-2 focus-visible:-outline-offset-2
                 focus-visible:outline-accent"
          :class="[
            !selectionMode && c.id === selectedId ? 'shadow-neu-in' : '',
            selectionMode ? 'pr-10' : (stateOf(c.id) === 'active' ? 'pr-16' : 'pr-10'),
          ]"
          :aria-current="!selectionMode && c.id === selectedId ? 'true' : undefined"
          @click="selectionMode ? toggleSelected(c.id) : emit('select', c.id)"
        >
          <span class="flex items-center gap-2 text-sm font-semibold text-ink">
            <input
              v-if="selectionMode"
              type="checkbox"
              class="size-4 shrink-0 cursor-pointer accent-accent-strong"
              :checked="selectedIds.has(c.id)"
              tabindex="-1"
              :aria-label="`Select ${c.label}`"
              @click.stop="toggleSelected(c.id)"
            />
            <StatusDot v-else :state="stateOf(c.id)"/>
            <span class="truncate">{{ c.label }}</span>
          </span>
          <span class="mt-0.5 block truncate font-mono text-xs text-ink-muted">
            {{ c.username }}@{{ c.host }}:{{ c.sshPort }}
          </span>
          <span v-if="c.tags.length" class="mt-1.5 flex min-w-0 items-center gap-1">
            <TagChip
              v-for="tag in c.tags.slice(0, ROW_TAGS)"
              :key="tag.id"
              class="min-w-0"
              :label="tag.label"
              :color="tag.color"
            />
            <span
              v-if="c.tags.length > ROW_TAGS"
              class="shrink-0 text-xs text-ink-faint"
              :title="c.tags.slice(ROW_TAGS).map(t => t.label).join(', ')"
            >+{{ c.tags.length - ROW_TAGS }}</span>
          </span>
        </button>

        <!-- Absolutely positioned rather than nested, so these are not
             buttons inside a button. Hidden in selection mode: neither
             action makes sense while picking connections to delete. -->
        <div v-if="!selectionMode" class="absolute top-2.5 right-2 flex items-center gap-1">
          <!-- Only offered while the tunnel is up: otherwise the browser
               would just land on a refused connection. -->
          <button
            v-if="stateOf(c.id) === 'active'"
            type="button"
            class="inline-flex cursor-pointer items-center justify-center
                   rounded-lg p-1.5 text-ink-muted
                   transition-colors duration-150 outline-none
                   hover:bg-hover hover:text-accent-ink
                   focus-visible:outline-2 focus-visible:outline-offset-1
                   focus-visible:outline-accent"
            :title="`Open http://localhost:${c.localPort} in your browser`"
            :aria-label="`Open ${c.label} in your browser`"
            @click.stop="openInBrowser(c.id)"
          >
            <ArrowSquareOut :size="15"/>
          </button>

          <button
            type="button"
            class="inline-flex cursor-pointer items-center justify-center
                   rounded-lg p-1.5 transition-colors duration-150 outline-none
                   hover:bg-hover
                   focus-visible:outline-2 focus-visible:outline-offset-1
                   focus-visible:outline-accent
                   disabled:cursor-not-allowed"
            :class="stateOf(c.id) === 'active'
              ? 'text-state-active hover:text-state-active'
              : stateOf(c.id) === 'error'
                ? 'text-state-error hover:text-state-error'
                : 'text-ink-muted hover:text-accent-ink'"
            :disabled="stateOf(c.id) === 'connecting'"
            :title="stateOf(c.id) === 'active' ? `Stop ${c.label}` : `Start ${c.label}`"
            :aria-label="stateOf(c.id) === 'active' ? `Stop ${c.label}` : `Start ${c.label}`"
            @click.stop="toggleTunnel(c.id)"
          >
            <CircleNotch
              v-if="stateOf(c.id) === 'connecting'"
              :size="15"
              class="animate-spin motion-reduce:animate-none"
            />
            <Power v-else :size="15" :weight="stateOf(c.id) === 'active' ? 'fill' : 'regular'"/>
          </button>
        </div>
      </li>
    </ul>

    <ConfirmDialog
      v-if="confirmingBulkDelete"
      title="Delete connections"
      :message="`Delete ${selectedIds.size} connection${selectedIds.size === 1 ? '' : 's'}? This can't be undone.`"
      @confirm="confirmBulkDelete"
      @cancel="confirmingBulkDelete = false"
    />
  </aside>
</template>
