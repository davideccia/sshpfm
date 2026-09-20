<script lang="ts" setup>
import Button from './ui/Button.vue'
import Dialog from './ui/Dialog.vue'

defineProps<{
  title: string
  message: string
  confirmLabel?: string
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <Dialog :title="title" width="sm" @cancel="emit('cancel')">
    <p class="text-sm leading-relaxed text-ink-muted">{{ message }}</p>

    <template #actions>
      <Button variant="secondary" @click="emit('cancel')">Cancel</Button>
      <Button variant="danger" @click="emit('confirm')">{{ confirmLabel ?? 'Delete' }}</Button>
    </template>
  </Dialog>
</template>
