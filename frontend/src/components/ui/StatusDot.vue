<script lang="ts" setup>
import {computed} from 'vue'
import type {TunnelState} from '../../composables/useTunnels'

const props = defineProps<{
  state: TunnelState
}>()

const labels: Record<TunnelState, string> = {
  stopped: 'Stopped',
  connecting: 'Connecting',
  active: 'Active',
  error: 'Error',
}

const styles: Record<TunnelState, string> = {
  stopped: 'bg-state-stopped',
  connecting: 'bg-state-connecting animate-dot-pulse motion-reduce:animate-none',
  active: 'bg-state-active',
  error: 'bg-state-error',
}

const label = computed(() => labels[props.state] ?? 'Unknown')
const style = computed(() => styles[props.state] ?? 'bg-state-stopped')
</script>

<template>
  <span
    class="inline-block size-2 shrink-0 rounded-full shadow-neu-out-sm"
    :class="style"
    role="img"
    :aria-label="label"
    :title="label"
  />
</template>
