<script lang="ts" setup>
import {computed, nextTick, ref, watch} from 'vue'
import {PhTerminalWindow as TerminalWindow} from '@phosphor-icons/vue'
import {useTunnelLogs} from '../composables/useTunnelLogs'

const props = defineProps<{
  connectionId: number | null
  isUp: boolean
}>()

const {logsOf, load} = useTunnelLogs()
const lines = computed(() => logsOf(props.connectionId))
const viewport = ref<HTMLElement | null>(null)

function scrollToBottom() {
  if (viewport.value) viewport.value.scrollTop = viewport.value.scrollHeight
}

// Backfills history on open/switch — the live event stream alone would
// miss lines logged before this panel existed.
watch(
  () => props.connectionId,
  async id => {
    if (id !== null) await load(id)
    await nextTick()
    scrollToBottom()
  },
  {immediate: true},
)

watch(lines, async () => {
  await nextTick()
  scrollToBottom()
})

function levelClass(level: string) {
  if (level === 'error') return 'text-state-error'
  if (level === 'warn') return 'text-state-connecting'
  return 'text-ink-muted'
}
</script>

<template>
  <div
    v-if="lines.length > 0"
    ref="viewport"
    class="h-[420px] overflow-y-auto rounded-2xl bg-canvas p-4 font-mono text-xs shadow-neu-in"
  >
    <div v-for="(line, i) in lines" :key="i" class="whitespace-pre-wrap" :class="levelClass(line.level)">
      <span class="text-ink-faint">{{ line.time.slice(11, 19) }}</span> {{ line.message }}
    </div>
  </div>

  <div
    v-else
    class="flex h-[420px] flex-col items-center justify-center gap-3 rounded-2xl bg-canvas text-ink-faint shadow-neu-in"
  >
    <TerminalWindow :size="40" weight="light"/>
    <p class="m-0 text-sm">{{ isUp ? 'No log activity yet.' : 'Connect to see logs here.' }}</p>
  </div>
</template>
