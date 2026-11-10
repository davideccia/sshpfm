<script lang="ts" setup>
import {PhX as X} from '@phosphor-icons/vue'

/**
 * A Tag as an inline data chip. The user's colour is painted only on the
 * dot: the label stays `text-ink` on the canvas, so no colour the user
 * picks can make it unreadable.
 */
defineProps<{
  label: string
  color: string
  removable?: boolean
}>()

const emit = defineEmits<{
  remove: []
}>()
</script>

<template>
  <span
    class="inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-lg bg-canvas
           px-2 py-0.5 text-xs text-ink shadow-neu-in-sm"
  >
    <span class="size-2 shrink-0 rounded-full" :style="{backgroundColor: color}" aria-hidden="true"/>
    <span class="truncate">{{ label }}</span>
    <button
      v-if="removable"
      type="button"
      class="-mr-1 inline-flex shrink-0 cursor-pointer items-center justify-center
             rounded-md p-0.5 text-ink-faint transition-colors duration-150
             outline-none hover:text-ink focus-visible:outline-2
             focus-visible:outline-offset-1 focus-visible:outline-accent"
      :aria-label="`Remove tag ${label}`"
      @click="emit('remove')"
    >
      <X :size="10" weight="bold"/>
    </button>
  </span>
</template>
