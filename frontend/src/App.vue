<script lang="ts" setup>
import {ref, computed, watch, onMounted} from 'vue'
import {PhWarningCircle as WarningCircle} from '@phosphor-icons/vue'
import ConnectionSidebar from './components/ConnectionSidebar.vue'
import ConnectionDetail from './components/ConnectionDetail.vue'
import SettingsView from './components/SettingsView.vue'
import VaultGate from './components/VaultGate.vue'
import {useConnections, type ConnectionDraft} from './composables/useConnections'
import {useTunnels} from './composables/useTunnels'
import {useVault} from './composables/useVault'
import {StartupError} from '../wailsjs/go/main/App'
import type {store} from '../wailsjs/go/models'

const {connections, isLoading, error, refresh, create, update, remove} = useConnections()
const {init: initTunnels, error: tunnelError} = useTunnels()
const {status: vaultStatus, refresh: refreshVault} = useVault()

type Mode = 'view' | 'create' | 'edit' | 'settings'

const mode = ref<Mode>('view')
const selectedId = ref<number | null>(null)
const startupError = ref('')

const selected = computed<store.Connection | null>(() =>
  connections.value.find(c => c.id === selectedId.value) ?? null
)

// The sidebar shows one message; tunnel failures are the more urgent of
// the two, so they win.
const banner = computed(() => tunnelError.value || error.value)

function selectConnection(id: number) {
  selectedId.value = id
  mode.value = 'view'
}

function startCreate() {
  selectedId.value = null
  mode.value = 'create'
}

function openSettings() {
  mode.value = 'settings'
}

function startEdit() {
  mode.value = 'edit'
}

function cancelForm() {
  mode.value = 'view'
}

async function handleCreate(value: ConnectionDraft) {
  try {
    const id = await create(value)
    selectedId.value = id
    mode.value = 'view'
  } catch {
    // useConnections already surfaced the message; keep the form open so
    // the user does not lose what they typed.
  }
}

async function handleUpdate(value: store.Connection) {
  try {
    await update(value)
    mode.value = 'view'
  } catch {
    // Same as create: stay on the form.
  }
}

async function handleDelete(id: number) {
  try {
    await remove(id)
    if (selectedId.value === id) selectedId.value = null
    mode.value = 'view'
  } catch {
    // The error is already visible in the sidebar.
  }
}

// Every binding below the gate refuses to answer while the vault is shut,
// so the data load hangs off the transition into `unlocked` rather than
// off mount. That covers both unlocking and setting the password up.
watch(vaultStatus, async (status) => {
  if (status !== 'unlocked') return
  await refresh()
  await initTunnels()
})

onMounted(async () => {
  startupError.value = await StartupError().catch(e => String(e))
  if (startupError.value) return

  await refreshVault()
})
</script>

<template>
  <div class="flex h-screen flex-col bg-canvas">
    <!-- Once the sidebar is on screen it carries the wordmark itself, right
         above "Connections", so the two don't read as separate bands. This
         header only covers the states before that: startup failure and the
         vault gate. -->
    <header
      v-if="vaultStatus !== 'unlocked'"
      class="flex h-14 shrink-0 items-center bg-canvas px-4"
    >
      <h1 class="m-0 font-display text-lg tracking-wide text-ink select-none">sshpfm</h1>
    </header>

    <p
      v-if="startupError"
      role="alert"
      class="m-8 flex items-start gap-2 rounded-lg border border-danger
             bg-danger-wash p-4 text-sm leading-relaxed text-danger"
    >
      <WarningCircle :size="18" weight="fill" class="mt-0.5 shrink-0"/>
      {{ startupError }}
    </p>

    <!-- `unknown` renders nothing: the gate would otherwise flash the
         setup form for a frame before the real state arrives. -->
    <VaultGate v-else-if="vaultStatus === 'unset' || vaultStatus === 'locked'"/>

    <div v-else-if="vaultStatus === 'unlocked'" class="flex min-h-0 flex-1">
      <ConnectionSidebar
        :connections="connections"
        :selected-id="mode === 'settings' ? null : selectedId"
        :settings-active="mode === 'settings'"
        :is-loading="isLoading"
        :error="banner"
        @select="selectConnection"
        @create="startCreate"
        @settings="openSettings"
      />
      <SettingsView
        v-if="mode === 'settings'"
        @imported="refresh"
        @close="cancelForm"
      />
      <ConnectionDetail
        v-else
        :mode="mode"
        :selected="selected"
        @create="handleCreate"
        @update="handleUpdate"
        @delete="handleDelete"
        @edit="startEdit"
        @cancel="cancelForm"
      />
    </div>
  </div>
</template>
