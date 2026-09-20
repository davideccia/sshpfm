<script lang="ts" setup>
import {ref, computed, watch} from 'vue'
import {
  PhPlugs as Plugs,
  PhPencilSimple as PencilSimple,
  PhTrash as Trash,
  PhPlay as Play,
  PhStop as Stop,
  PhWarningCircle as WarningCircle,
} from '@phosphor-icons/vue'
import ConnectionForm from './ConnectionForm.vue'
import ConnectionLogPanel from './ConnectionLogPanel.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import HostKeyDialog from './HostKeyDialog.vue'
import Button from './ui/Button.vue'
import StatusDot from './ui/StatusDot.vue'
import {useTunnels} from '../composables/useTunnels'
import type {ConnectionDraft} from '../composables/useConnections'
import type {store} from '../../wailsjs/go/models'

type Mode = 'view' | 'create' | 'edit'

const props = defineProps<{
  mode: Mode
  selected: store.Connection | null
}>()

const emit = defineEmits<{
  create: [value: ConnectionDraft]
  update: [value: store.Connection]
  delete: [id: number]
  edit: []
  cancel: []
}>()

const {statusOf, stateOf, start, stop, trustHostKey} = useTunnels()

const confirmingDelete = ref(false)
const activeTab = ref<'overview' | 'log'>('overview')
watch(
  () => props.selected?.id,
  () => {
    activeTab.value = 'overview'
  },
)

const status = computed(() => (props.selected ? statusOf(props.selected.id) : null))
const state = computed(() => (props.selected ? stateOf(props.selected.id) : 'stopped'))
// Connecting counts as up: a handshake that hangs has to be cancellable,
// so the button stays live and reads "Stop" from the first click onward.
const isUp = computed(() => state.value === 'active' || state.value === 'connecting')

// Shown only while the tunnel is stopped on an untrusted host key.
const pendingHostKey = computed(() => status.value?.pendingHostKey ?? null)

const stateLabel = computed(() => {
  switch (state.value) {
    case 'active':
      return status.value?.streams
        ? `Active · ${status.value.streams} stream${status.value.streams === 1 ? '' : 's'}`
        : 'Active · idle'
    case 'connecting':
      return 'Connecting…'
    case 'error':
      return 'Error'
    default:
      return 'Stopped'
  }
})

function toggleTunnel() {
  if (!props.selected) return
  if (isUp.value) stop(props.selected.id)
  else start(props.selected.id)
}

function handleUpdate(value: ConnectionDraft) {
  if (!props.selected) return
  // hasPassword is read-only on the backend; sending the current value
  // back keeps the payload a complete Connection without claiming
  // anything the store will act on.
  emit('update', {id: props.selected.id, hasPassword: props.selected.hasPassword, ...value})
}

function confirmDelete() {
  confirmingDelete.value = false
  if (props.selected) emit('delete', props.selected.id)
}

function acceptHostKey() {
  if (props.selected) trustHostKey(props.selected.id)
}

// Dismissing the prompt must clear the pending key, otherwise the dialog
// reappears on every render. Dropping the failed tunnel does both.
function dismissHostKey() {
  if (props.selected) stop(props.selected.id)
}
</script>

<template>
  <section class="flex-1 overflow-y-auto bg-canvas p-8">
    <ConnectionForm
      v-if="mode === 'create'"
      submit-label="Create"
      @submit="value => emit('create', value)"
      @cancel="emit('cancel')"
    />

    <ConnectionForm
      v-else-if="mode === 'edit' && selected"
      :key="selected.id"
      :initial="selected"
      submit-label="Save"
      @submit="handleUpdate"
      @cancel="emit('cancel')"
    />

    <div v-else-if="selected" class="flex max-w-[560px] flex-col gap-4">
      <div class="flex items-center justify-between gap-4">
        <h2 class="truncate text-xl font-semibold text-ink">{{ selected.label }}</h2>
        <div class="flex shrink-0 gap-2">
          <!-- Stopping is routine and reversible, so it stays neutral.
               Red is reserved for Delete: two red buttons side by side
               made the destructive one easy to hit by mistake. -->
          <Button
            :variant="isUp ? 'secondary' : 'primary'"
            @click="toggleTunnel"
          >
            <Stop v-if="isUp" :size="16" weight="fill"/>
            <Play v-else :size="16" weight="fill"/>
            {{ isUp ? 'Stop' : 'Start' }}
          </Button>
          <Button
            variant="secondary"
            :disabled="isUp"
            :title="isUp ? 'Stop the tunnel before editing' : undefined"
            @click="emit('edit')"
          >
            <PencilSimple :size="16"/>
            Edit
          </Button>
          <Button variant="danger" @click="confirmingDelete = true">
            <Trash :size="16"/>
            Delete
          </Button>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2 text-sm">
        <StatusDot :state="state"/>
        <span class="font-semibold text-ink">{{ stateLabel }}</span>
        <code
          v-if="state === 'active'"
          class="rounded-lg bg-canvas px-2 py-1 font-mono text-xs text-ink-muted shadow-neu-in-sm"
        >
          localhost:{{ selected.localPort }} → {{ selected.host }}:{{ selected.remoteBindPort }}
        </code>
      </div>

      <p
        v-if="status?.error"
        role="alert"
        class="flex items-start gap-2 rounded-lg border border-danger/40
               bg-danger-wash p-3 text-xs leading-relaxed text-danger"
      >
        <WarningCircle :size="16" weight="fill" class="mt-px shrink-0"/>
        {{ status.error }}
      </p>

      <div class="flex gap-2">
        <Button :variant="activeTab === 'overview' ? 'primary' : 'secondary'" @click="activeTab = 'overview'">
          Overview
        </Button>
        <Button :variant="activeTab === 'log' ? 'primary' : 'secondary'" @click="activeTab = 'log'">
          Log
        </Button>
      </div>

      <!-- Six fields in two columns. Host and Auth span a row each: they
           hold the longest values, and the remaining four fill two full
           rows between them. -->
      <dl
        v-if="activeTab === 'overview'"
        class="m-0 grid grid-cols-2 gap-4 rounded-2xl bg-canvas p-4 shadow-neu-out"
      >
        <div class="col-span-2">
          <dt class="mb-0.5 text-xs text-ink-muted">Host</dt>
          <dd class="m-0 truncate font-mono text-sm text-ink">{{ selected.host }}</dd>
        </div>
        <div>
          <dt class="mb-0.5 text-xs text-ink-muted">Username</dt>
          <dd class="m-0 truncate font-mono text-sm text-ink">{{ selected.username }}</dd>
        </div>
        <div>
          <dt class="mb-0.5 text-xs text-ink-muted">SSH port</dt>
          <dd class="m-0 font-mono text-sm text-ink">{{ selected.sshPort }}</dd>
        </div>
        <div>
          <dt class="mb-0.5 text-xs text-ink-muted">Local port</dt>
          <dd class="m-0 font-mono text-sm text-ink">{{ selected.localPort }}</dd>
        </div>
        <div>
          <dt class="mb-0.5 text-xs text-ink-muted">Remote bind port</dt>
          <dd class="m-0 font-mono text-sm text-ink">{{ selected.remoteBindPort }}</dd>
        </div>
        <!-- Whether a password is stored, never the password itself: the
             backend redacts it before it reaches the frontend. -->
        <div class="col-span-2">
          <dt class="mb-0.5 text-xs text-ink-muted">Auth</dt>
          <dd class="m-0 text-sm text-ink">
            {{ selected.hasPassword ? 'Agent key, then saved password' : 'Agent key' }}
          </dd>
        </div>
      </dl>

      <ConnectionLogPanel v-else :connection-id="selected.id" :is-up="isUp"/>
    </div>

    <div v-else class="flex h-full flex-col items-center justify-center gap-3 text-ink-faint">
      <Plugs :size="40" weight="light"/>
      <p class="m-0 text-sm">Select a connection, or create a new one.</p>
    </div>

    <ConfirmDialog
      v-if="confirmingDelete && selected"
      title="Delete connection"
      :message="`Delete connection '${selected.label}'? This can't be undone.`"
      confirm-label="Delete"
      @confirm="confirmDelete"
      @cancel="confirmingDelete = false"
    />

    <HostKeyDialog
      v-if="pendingHostKey"
      :pending="pendingHostKey"
      @trust="acceptHostKey"
      @cancel="dismissHostKey"
    />
  </section>
</template>
