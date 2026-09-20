<script lang="ts" setup>
import {
  PhPlus as Plus,
  PhWarningCircle as WarningCircle,
  PhArrowSquareOut as ArrowSquareOut,
} from '@phosphor-icons/vue'
import Button from './ui/Button.vue'
import StatusDot from './ui/StatusDot.vue'
import {useTunnels} from '../composables/useTunnels'
import type {store} from '../../wailsjs/go/models'

defineProps<{
  connections: store.Connection[]
  selectedId: number | null
  isLoading: boolean
  error: string
}>()

const {stateOf, openInBrowser} = useTunnels()

const emit = defineEmits<{
  select: [id: number]
  create: []
}>()
</script>

<template>
  <aside
    class="flex w-[300px] shrink-0 flex-col gap-4 overflow-y-auto
           border-r border-line bg-surface px-4 py-5"
  >
    <div class="flex items-center justify-between">
      <h2 class="text-base font-semibold text-ink">Connections</h2>
      <Button variant="primary" @click="emit('create')">
        <Plus :size="16" weight="bold"/>
        New
      </Button>
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

    <ul v-else class="flex list-none flex-col gap-1">
      <li v-for="c in connections" :key="c.id" class="relative">
        <button
          type="button"
          class="relative w-full cursor-pointer rounded-lg px-3 py-2.5 text-left
                 transition-colors duration-150 outline-none
                 hover:bg-raised
                 focus-visible:outline-2 focus-visible:-outline-offset-2
                 focus-visible:outline-accent
                 before:absolute before:inset-y-2 before:left-0 before:w-0.5
                 before:rounded-full before:bg-accent before:opacity-0
                 before:transition-opacity before:duration-150"
          :class="[
            c.id === selectedId ? 'bg-raised before:opacity-100' : '',
            stateOf(c.id) === 'active' ? 'pr-10' : '',
          ]"
          :aria-current="c.id === selectedId ? 'true' : undefined"
          @click="emit('select', c.id)"
        >
          <span class="flex items-center gap-2 text-sm font-semibold text-ink">
            <StatusDot :state="stateOf(c.id)"/>
            <span class="truncate">{{ c.label }}</span>
          </span>
          <span class="mt-0.5 block truncate font-mono text-xs text-ink-muted">
            {{ c.username }}@{{ c.host }}:{{ c.sshPort }}
          </span>
        </button>

        <!-- Only offered while the tunnel is up: otherwise the browser would
             just land on a refused connection. Absolutely positioned rather
             than nested, so it is not a button inside a button. -->
        <button
          v-if="stateOf(c.id) === 'active'"
          type="button"
          class="absolute top-2.5 right-2 inline-flex cursor-pointer items-center
                 justify-center rounded-lg p-1.5 text-ink-muted
                 transition-colors duration-150 outline-none
                 hover:bg-hover hover:text-accent-ink
                 focus-visible:outline-2 focus-visible:outline-offset-1
                 focus-visible:outline-accent"
          :title="`Open http://localhost:${c.localPort} in your browser`"
          :aria-label="`Open ${c.label} in your browser`"
          @click="openInBrowser(c.id)"
        >
          <ArrowSquareOut :size="15"/>
        </button>
      </li>
    </ul>
  </aside>
</template>
