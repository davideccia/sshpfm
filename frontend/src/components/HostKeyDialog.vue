<script lang="ts" setup>
import {PhShieldWarning as ShieldWarning} from '@phosphor-icons/vue'
import Button from './ui/Button.vue'
import Dialog from './ui/Dialog.vue'
import type {tunnel} from '../../wailsjs/go/models'

defineProps<{
  pending: tunnel.PendingHostKey
}>()

const emit = defineEmits<{
  trust: []
  cancel: []
}>()
</script>

<template>
  <Dialog title="Unknown host" width="md" tone="warning" @cancel="emit('cancel')">
    <template #icon>
      <ShieldWarning :size="20" weight="fill"/>
    </template>

    <p class="text-sm leading-relaxed text-ink-muted">
      <code class="font-mono text-ink">{{ pending.host }}</code> is not in your
      <code class="font-mono text-ink">known_hosts</code> file. Confirm this
      fingerprint out of band before trusting it. Accepting the wrong key lets
      someone read everything sent through the tunnel.
    </p>

    <dl class="mt-4 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded-lg
               border border-line bg-raised p-3">
      <dt class="text-xs text-ink-muted">Key type</dt>
      <dd class="m-0 font-mono text-xs text-ink">{{ pending.keyType }}</dd>

      <dt class="text-xs text-ink-muted">Fingerprint</dt>
      <!-- Fingerprints are long; wrapping beats truncating the part that
           differs between a real host and an impostor. -->
      <dd class="m-0 font-mono text-xs break-all text-ink">{{ pending.fingerprint }}</dd>
    </dl>

    <template #actions>
      <Button variant="secondary" @click="emit('cancel')">Cancel</Button>
      <Button variant="primary" @click="emit('trust')">Trust &amp; connect</Button>
    </template>
  </Dialog>
</template>
