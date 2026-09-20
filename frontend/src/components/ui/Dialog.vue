<script lang="ts" setup>
import {computed, onBeforeUnmount, onMounted, ref, useId} from 'vue'

const props = withDefaults(defineProps<{
  title: string
  // The two sizes the app actually needs: a confirmation, and a panel
  // holding a full host key.
  width?: 'sm' | 'md'
  // Colours the title only. Used by the host-key prompt, which is a
  // warning rather than a neutral question.
  tone?: 'neutral' | 'warning'
}>(), {
  width: 'sm',
  tone: 'neutral',
})

const emit = defineEmits<{
  cancel: []
}>()

const panel = ref<HTMLElement | null>(null)
const titleId = `${useId()}-title`

const widthClass = computed(() =>
  props.width === 'md' ? 'w-[min(460px,90vw)]' : 'w-[min(360px,90vw)]'
)

const toneClass = computed(() =>
  props.tone === 'warning' ? 'text-state-connecting' : 'text-ink'
)

const FOCUSABLE = 'a[href], button:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex="-1"])'

let restoreFocusTo: HTMLElement | null = null

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    emit('cancel')
    return
  }
  if (e.key !== 'Tab' || !panel.value) return

  // Without this, Tab walks out of the dialog and onto the buttons behind
  // the scrim, which are still clickable by keyboard.
  const items = Array.from(panel.value.querySelectorAll<HTMLElement>(FOCUSABLE))
  if (items.length === 0) {
    e.preventDefault()
    panel.value.focus()
    return
  }

  const first = items[0]
  const last = items[items.length - 1]
  const active = document.activeElement

  if (e.shiftKey && (active === first || active === panel.value)) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && active === last) {
    e.preventDefault()
    first.focus()
  }
}

onMounted(() => {
  restoreFocusTo = document.activeElement as HTMLElement | null
  document.addEventListener('keydown', onKeydown)
  // Move focus into the dialog so the next Tab lands on its buttons rather
  // than continuing through the page behind the scrim.
  panel.value?.focus()
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  // Send focus back to whatever opened the dialog, so keyboard users do
  // not restart from the top of the document.
  restoreFocusTo?.focus()
})
</script>

<template>
  <!-- Teleported because the dialog is declared inside the scrolling
       detail pane; leaving it there puts modal content in the middle of
       that region's reading order. -->
  <Teleport to="body">
    <!-- The only stacking layer in the app: nothing else sets a z-index. -->
    <div
      class="fixed inset-0 z-50 flex items-center justify-center bg-scrim p-4"
      @click.self="emit('cancel')"
    >
      <div
        ref="panel"
        class="rounded-xl border border-line bg-surface p-6 text-left shadow-2xl
               outline-none"
        :class="widthClass"
        role="alertdialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        tabindex="-1"
      >
        <h3
          :id="titleId"
          class="mb-3 flex items-center gap-2 text-base font-semibold"
          :class="toneClass"
        >
          <slot name="icon"/>
          {{ title }}
        </h3>

        <slot/>

        <div class="mt-6 flex justify-end gap-2">
          <slot name="actions"/>
        </div>
      </div>
    </div>
  </Teleport>
</template>
