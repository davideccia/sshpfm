<script lang="ts" setup>
import {reactive, computed, watch} from 'vue'
import TextField from './ui/TextField.vue'
import Button from './ui/Button.vue'
import type {ConnectionDraft} from '../composables/useConnections'
import type {store} from '../../wailsjs/go/models'

/**
 * Ports are nullable while editing. Seeding them with 0 showed the user a
 * value that fails validation before they have typed anything; null renders
 * as an empty field instead. `validate` is what turns them back into
 * numbers.
 *
 * `password` is always a string here and only becomes the backend's
 * three-way value on submit. `clearPassword` is the one way to remove a
 * stored secret, since an empty field has to mean "leave it alone".
 */
type FormState = Omit<ConnectionDraft, 'sshPort' | 'remoteBindPort' | 'localPort' | 'password'> & {
  sshPort: number | null
  remoteBindPort: number | null
  localPort: number | null
  password: string
  clearPassword: boolean
}

const props = defineProps<{
  initial?: store.Connection | null
  submitLabel: string
}>()

const emit = defineEmits<{
  submit: [value: ConnectionDraft]
  cancel: []
}>()

function stateFrom(initial?: store.Connection | null): FormState {
  return {
    label: initial?.label ?? '',
    host: initial?.host ?? '',
    // 22 is the answer for almost every host, so it is a useful default
    // rather than a placeholder the user has to fill in.
    sshPort: initial?.sshPort ?? 22,
    username: initial?.username ?? '',
    remoteBindPort: initial?.remoteBindPort ?? null,
    localPort: initial?.localPort ?? null,
    // Never seeded: the backend redacts the secret, so there is nothing
    // to seed it with, and an empty field is what "keep it" looks like.
    password: '',
    clearPassword: false,
  }
}

const form = reactive<FormState>(stateFrom(props.initial))

const errors = reactive<Record<string, string>>({})

watch(() => props.initial, (initial) => {
  Object.assign(form, stateFrom(initial))
})

function portError(value: number | null): string {
  if (value === null) return 'Required'
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    return 'Must be between 1 and 65535'
  }
  return ''
}

function validate(): boolean {
  Object.keys(errors).forEach(key => delete errors[key])

  if (!form.label.trim()) errors.label = 'Label is required'
  if (!form.host.trim()) errors.host = 'Host is required'
  if (!form.username.trim()) errors.username = 'Username is required'

  const ports = {
    sshPort: form.sshPort,
    localPort: form.localPort,
    remoteBindPort: form.remoteBindPort,
  }
  for (const [field, value] of Object.entries(ports)) {
    const message = portError(value)
    if (message) errors[field] = message
  }

  return Object.keys(errors).length === 0
}

const hasStoredPassword = computed(() => props.initial?.hasPassword ?? false)

const passwordPlaceholder = computed(() =>
  hasStoredPassword.value
    ? 'Saved — leave empty to keep it'
    : 'Optional — the agent key is tried first'
)

/**
 * The backend reads three cases from the *string it receives: omitted
 * leaves the stored password alone, an empty string removes it, and
 * anything else replaces it. `undefined` is dropped by the bridge's
 * JSON encoding, which is what produces the nil pointer.
 */
function passwordValue(): string | undefined {
  if (form.clearPassword) return ''
  return form.password === '' ? undefined : form.password
}

function handleSubmit() {
  if (!validate()) return
  emit('submit', {
    label: form.label.trim(),
    host: form.host.trim(),
    username: form.username.trim(),
    // validate() has already rejected null for all three.
    sshPort: form.sshPort as number,
    localPort: form.localPort as number,
    remoteBindPort: form.remoteBindPort as number,
    password: passwordValue(),
  })
}
</script>

<template>
  <form class="flex max-w-[460px] flex-col gap-4" @submit.prevent="handleSubmit">
    <h2 class="text-xl font-semibold text-ink">
      {{ initial ? `Edit ${initial.label}` : 'New connection' }}
    </h2>

    <TextField
      label="Label"
      v-model="form.label"
      placeholder="Acme staging database"
      required
      :error="errors.label"
    />
    <TextField
      label="Host"
      v-model="form.host"
      placeholder="bastion.example.com"
      required
      :error="errors.host"
    />
    <TextField
      label="Username"
      v-model="form.username"
      required
      :error="errors.username"
    />

    <div class="flex flex-col gap-1">
      <TextField
        label="Password"
        type="password"
        v-model="form.password"
        :placeholder="passwordPlaceholder"
        :disabled="form.clearPassword"
      />
      <!-- An empty field means "keep the saved password", so removing one
           needs its own control. -->
      <button
        v-if="hasStoredPassword"
        type="button"
        class="self-start text-xs underline-offset-2
               transition-colors duration-150 hover:underline
               focus-visible:outline-2 focus-visible:outline-offset-2
               focus-visible:outline-accent"
        :class="form.clearPassword ? 'text-danger' : 'text-ink-faint hover:text-ink-muted'"
        @click="form.clearPassword = !form.clearPassword"
      >
        {{ form.clearPassword
          ? 'Will be removed on save — undo'
          : 'Remove the saved password' }}
      </button>
    </div>

    <div class="grid grid-cols-3 gap-3">
      <TextField
        label="SSH port"
        type="number"
        :min="1"
        :max="65535"
        :step="1"
        v-model="form.sshPort"
        required
        :error="errors.sshPort"
      />
      <TextField
        label="Local port"
        type="number"
        :min="1"
        :max="65535"
        :step="1"
        v-model="form.localPort"
        required
        :error="errors.localPort"
      />
      <TextField
        label="Remote bind port"
        type="number"
        :min="1"
        :max="65535"
        :step="1"
        v-model="form.remoteBindPort"
        required
        :error="errors.remoteBindPort"
      />
    </div>

    <div class="mt-2 flex justify-end gap-2">
      <Button variant="secondary" type="button" @click="emit('cancel')">Cancel</Button>
      <Button variant="primary" type="submit">{{ submitLabel }}</Button>
    </div>
  </form>
</template>
