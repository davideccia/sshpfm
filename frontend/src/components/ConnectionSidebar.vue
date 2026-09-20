<script lang="ts" setup>
import {
  PhPlus as Plus,
  PhWarningCircle as WarningCircle,
  PhArrowSquareOut as ArrowSquareOut,
  PhPower as Power,
  PhCircleNotch as CircleNotch,
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

const {stateOf, start, stop, openInBrowser} = useTunnels()

const emit = defineEmits<{
  select: [id: number]
  create: []
}>()

function toggleTunnel(id: number) {
  stateOf(id) === 'active' ? stop(id) : start(id)
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
          class="relative w-full cursor-pointer rounded-xl px-3 py-2.5 text-left
                 transition-[box-shadow] duration-150 outline-none
                 hover:shadow-neu-out-sm
                 focus-visible:outline-2 focus-visible:-outline-offset-2
                 focus-visible:outline-accent"
          :class="[
            c.id === selectedId ? 'shadow-neu-in' : '',
            stateOf(c.id) === 'active' ? 'pr-16' : 'pr-10',
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

        <!-- Absolutely positioned rather than nested, so these are not
             buttons inside a button. -->
        <div class="absolute top-2.5 right-2 flex items-center gap-1">
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
  </aside>
</template>
