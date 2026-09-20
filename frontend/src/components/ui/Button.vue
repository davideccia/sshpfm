<script lang="ts" setup>
import {computed} from 'vue'

const props = withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'danger'
  type?: 'button' | 'submit'
}>(), {
  variant: 'secondary',
  type: 'button',
})

// Filled variants use the -strong step rather than the brand step: white on
// purple-500 is 3.96:1 and misses the 4.5:1 floor, white on purple-600 is
// 5.4:1. The brand step is then free to act as the hover.
const variants: Record<NonNullable<typeof props.variant>, string> = {
  primary: 'bg-accent-strong text-white not-disabled:hover:bg-accent',
  secondary: 'bg-raised text-ink border-line-strong not-disabled:hover:bg-hover',
  danger: 'bg-danger-strong text-white not-disabled:hover:bg-danger',
}

const variantClass = computed(() => variants[props.variant])
</script>

<template>
  <button
    :type="type"
    class="inline-flex shrink-0 cursor-pointer items-center justify-center gap-2
           whitespace-nowrap rounded-lg border border-transparent px-4 py-2
           text-sm font-semibold transition-colors duration-150
           outline-none focus-visible:outline-2 focus-visible:outline-offset-2
           focus-visible:outline-accent
           not-disabled:active:scale-[0.98]
           disabled:cursor-not-allowed disabled:opacity-40"
    :class="variantClass"
  >
    <slot/>
  </button>
</template>
