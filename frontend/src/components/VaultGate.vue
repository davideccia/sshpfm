<script lang="ts" setup>
import {ref, computed, watch, nextTick, useTemplateRef} from 'vue'
import {PhLock as Lock, PhWarningCircle as WarningCircle} from '@phosphor-icons/vue'
import TextField from './ui/TextField.vue'
import Button from './ui/Button.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import {useVault} from '../composables/useVault'

// Mirrors vault.MinPasswordLen on the Go side, which is the rule that
// actually holds; this copy only spares the user a round trip.
const MIN_LENGTH = 8

const {status, error, isBusy, create, unlock, reset} = useVault()

const password = ref('')
const confirmation = ref('')
const localError = ref('')
const confirmingReset = ref(false)

const isSetup = computed(() => status.value === 'unset')
const form = useTemplateRef<HTMLFormElement>('form')

// The gate is the only thing on screen, so the field should already have
// the caret when it appears.
watch(status, async () => {
  await nextTick()
  form.value?.querySelector('input')?.focus()
}, {immediate: true})

function validate(): boolean {
  localError.value = ''
  if (password.value.length < MIN_LENGTH) {
    localError.value = `Must be at least ${MIN_LENGTH} characters`
    return false
  }
  if (isSetup.value && password.value !== confirmation.value) {
    localError.value = 'The two passwords do not match'
    return false
  }
  return true
}

async function handleSubmit() {
  if (isBusy.value) return
  // On unlock the length rule is the backend's business: an old vault may
  // predate it, and rejecting the password here would lock the user out.
  if (isSetup.value && !validate()) return

  const ok = isSetup.value
    ? await create(password.value)
    : await unlock(password.value)

  // Clearing only on success keeps a mistyped password on screen to fix,
  // and never leaves the real one in a reactive ref afterwards.
  if (ok) {
    password.value = ''
    confirmation.value = ''
  }
}

async function handleReset() {
  confirmingReset.value = false
  password.value = ''
  confirmation.value = ''
  localError.value = ''
  await reset()
}

const message = computed(() => localError.value || error.value)
</script>

<template>
  <div class="flex min-h-0 flex-1 items-center justify-center p-8">
    <form
      ref="form"
      class="flex w-[min(380px,100%)] flex-col gap-4"
      @submit.prevent="handleSubmit"
    >
      <div class="flex flex-col items-center gap-2 text-center">
        <span
          class="flex size-11 items-center justify-center rounded-full
                 border border-line bg-raised text-accent-ink"
        >
          <Lock :size="20" weight="fill"/>
        </span>
        <h2 class="m-0 text-xl font-semibold text-ink">
          {{ isSetup ? 'Set a master password' : 'Unlock sshpfm' }}
        </h2>
        <p class="m-0 text-sm leading-relaxed text-ink-muted">
          <template v-if="isSetup">
            It encrypts the SSH passwords you save. It is never stored, so it
            cannot be recovered — lose it and those passwords are gone.
          </template>
          <template v-else>
            Enter your master password to decrypt your saved SSH passwords.
          </template>
        </p>
      </div>

      <TextField
        label="Master password"
        type="password"
        v-model="password"
        required
        :error="isSetup ? localError : ''"
      />
      <TextField
        v-if="isSetup"
        label="Confirm master password"
        type="password"
        v-model="confirmation"
        required
      />

      <p
        v-if="!isSetup && message"
        role="alert"
        class="m-0 flex items-start gap-2 text-sm text-danger"
      >
        <WarningCircle :size="16" weight="fill" class="mt-px shrink-0"/>
        {{ message }}
      </p>
      <p v-else-if="isSetup && error" role="alert" class="m-0 text-sm text-danger">
        {{ error }}
      </p>

      <!-- Deriving the key takes about 100ms of deliberate work; disabling
           the button stops a second attempt queueing behind the first. -->
      <Button variant="primary" type="submit" :disabled="isBusy">
        {{ isBusy ? 'Working…' : (isSetup ? 'Set password' : 'Unlock') }}
      </Button>

      <!-- Only offered on the unlock screen: during setup there is
           nothing yet to forget. -->
      <button
        v-if="!isSetup"
        type="button"
        class="mt-1 self-center text-xs text-ink-faint underline-offset-2
               transition-colors duration-150 hover:text-ink-muted hover:underline
               focus-visible:outline-2 focus-visible:outline-offset-2
               focus-visible:outline-accent"
        @click="confirmingReset = true"
      >
        Forgot your master password?
      </button>
    </form>

    <ConfirmDialog
      v-if="confirmingReset"
      title="Start over"
      message="This erases every saved SSH password. Your connections, hosts and ports are kept. This can't be undone."
      confirm-label="Erase passwords"
      @confirm="handleReset"
      @cancel="confirmingReset = false"
    />
  </div>
</template>
