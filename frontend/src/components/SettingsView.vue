<script lang="ts" setup>
import {ref} from 'vue'
import {
  PhArrowLeft as ArrowLeft,
  PhDownloadSimple as DownloadSimple,
  PhUploadSimple as UploadSimple,
  PhWarningCircle as WarningCircle,
  PhCheckCircle as CheckCircle,
} from '@phosphor-icons/vue'
import Button from './ui/Button.vue'
import {ExportDatabase, ImportDatabase} from '../../wailsjs/go/main/App'

const emit = defineEmits<{
  imported: []
  close: []
}>()

const isExporting = ref(false)
const isImporting = ref(false)
const error = ref('')
const notice = ref('')

async function handleExport() {
  if (isExporting.value) return
  error.value = ''
  notice.value = ''
  isExporting.value = true
  try {
    await ExportDatabase()
    notice.value = 'Connections exported.'
  } catch (e) {
    error.value = String(e)
  } finally {
    isExporting.value = false
  }
}

async function handleImport() {
  if (isImporting.value) return
  error.value = ''
  notice.value = ''
  isImporting.value = true
  try {
    await ImportDatabase()
    notice.value = 'Connections imported.'
    emit('imported')
  } catch (e) {
    error.value = String(e)
  } finally {
    isImporting.value = false
  }
}
</script>

<template>
  <section class="flex-1 overflow-y-auto bg-canvas p-8">
    <div class="flex max-w-[560px] flex-col gap-4">
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="inline-flex cursor-pointer items-center justify-center
                 rounded-lg p-1.5 text-ink-muted transition-colors duration-150
                 outline-none hover:bg-hover hover:text-accent-ink
                 focus-visible:outline-2 focus-visible:outline-offset-1
                 focus-visible:outline-accent"
          aria-label="Back"
          @click="emit('close')"
        >
          <ArrowLeft :size="18"/>
        </button>
        <h2 class="m-0 text-xl font-semibold text-ink">Settings</h2>
      </div>

      <div class="flex flex-col gap-3 rounded-2xl bg-canvas p-4 shadow-neu-out">
        <h3 class="m-0 text-sm font-semibold text-ink">Connections file</h3>
        <p class="m-0 text-sm leading-relaxed text-ink-muted">
          Export or import your connection profiles (host, ports, username) as
          a JSON file.
        </p>

        <p
          role="alert"
          class="m-0 flex items-start gap-2 rounded-lg border border-danger/40
                 bg-danger-wash p-3 text-xs leading-relaxed text-danger"
        >
          <WarningCircle :size="16" weight="fill" class="mt-px shrink-0"/>
          Saved SSH passwords are never included. After an import, add them
          back on each connection that needs one.
        </p>

        <div class="flex gap-2">
          <Button variant="secondary" :disabled="isExporting" @click="handleExport">
            <DownloadSimple :size="16"/>
            {{ isExporting ? 'Exporting…' : 'Export connections' }}
          </Button>
          <Button variant="secondary" :disabled="isImporting" @click="handleImport">
            <UploadSimple :size="16"/>
            {{ isImporting ? 'Importing…' : 'Import connections' }}
          </Button>
        </div>

        <p v-if="error" role="alert" class="m-0 flex items-start gap-2 text-xs leading-relaxed text-danger">
          <WarningCircle :size="14" weight="fill" class="mt-px shrink-0"/>
          {{ error }}
        </p>
        <p v-else-if="notice" class="m-0 flex items-start gap-2 text-xs leading-relaxed text-ink-muted">
          <CheckCircle :size="14" weight="fill" class="mt-px shrink-0 text-accent-ink"/>
          {{ notice }}
        </p>
      </div>
    </div>
  </section>
</template>
