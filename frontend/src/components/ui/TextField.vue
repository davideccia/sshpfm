<script lang="ts" setup>
import {computed, useId} from 'vue'

const props = withDefaults(defineProps<{
  label: string
  // `null` is an empty number field. Emitting NaN instead would put NaN
  // into the form model and render as an unparseable value attribute.
  modelValue: string | number | null
  type?: 'text' | 'number' | 'password'
  min?: number
  max?: number
  step?: number
  required?: boolean
  disabled?: boolean
  error?: string
  placeholder?: string
}>(), {
  type: 'text',
  required: false,
  disabled: false,
  error: '',
  placeholder: undefined,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | null]
}>()

function onInput(event: Event) {
  const input = event.target as HTMLInputElement
  if (props.type !== 'number') {
    emit('update:modelValue', input.value)
    return
  }
  // An emptied number field reports NaN; report it as null so the caller
  // can tell "not filled in" apart from a real value.
  emit('update:modelValue', Number.isNaN(input.valueAsNumber) ? null : input.valueAsNumber)
}

const errorId = `${useId()}-error`

// Ports and other numbers line up only in a monospace face, and every
// numeric field in this app is a port.
const inputFont = computed(() => props.type === 'number' ? 'font-mono' : 'font-sans')
</script>

<template>
  <label class="flex flex-col gap-1">
    <span class="text-xs font-semibold text-ink-muted">{{ label }}</span>
    <input
      class="w-full rounded-lg border bg-raised px-3 py-2 text-sm text-ink
             transition-colors duration-150 outline-none
             placeholder:text-ink-faint
             focus-visible:outline-2 focus-visible:outline-offset-2
             focus-visible:outline-accent
             disabled:cursor-not-allowed disabled:opacity-40"
      :class="[inputFont, error ? 'border-danger' : 'border-line-strong']"
      :type="type"
      :min="min"
      :max="max"
      :step="step"
      :required="required"
      :disabled="disabled"
      :value="modelValue ?? ''"
      :placeholder="placeholder"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="error ? errorId : undefined"
      @input="onInput"
    />
    <span v-if="error" :id="errorId" class="text-xs text-danger">{{ error }}</span>
  </label>
</template>
