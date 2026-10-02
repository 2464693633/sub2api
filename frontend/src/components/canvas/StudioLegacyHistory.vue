<template>
  <div v-if="available" class="mb-2 rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs dark:border-amber-800 dark:bg-amber-900/20">
    <p class="mb-1 text-amber-800 dark:text-amber-200">{{ t('canvas.legacyHistoryNotice') }}</p>
    <button type="button" :disabled="importing" class="font-medium text-primary-600 disabled:opacity-50 dark:text-primary-400" @click="recover">
      {{ t(importing ? 'canvas.legacyHistoryImporting' : 'canvas.legacyHistoryImport') }}
    </button>
    <p v-if="error" role="alert" class="mt-1 text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { hasLegacyStudioHistory, importLegacyStudioHistory, type StudioKind } from '@/composables/studioHistory'
import { sessionOwnerKey } from '@/composables/studioSession'

const props = defineProps<{ kind: StudioKind }>()
const emit = defineEmits<{ imported: [] }>()
const { t } = useI18n()
const available = ref(false)
const importing = ref(false)
const error = ref('')
let epoch = 0

function snapshot() {
  const owner = sessionOwnerKey()
  const currentEpoch = epoch
  return { owner, isCurrent: () => epoch === currentEpoch && sessionOwnerKey() === owner }
}

async function probe() {
  const session = snapshot()
  try {
    const found = await hasLegacyStudioHistory(props.kind, session.owner)
    if (session.isCurrent()) available.value = found
  } catch {
    // Recovery remains available to retry if storage access temporarily fails.
    if (session.isCurrent()) available.value = true
  }
}

async function recover() {
  if (importing.value) return
  const session = snapshot()
  if (!window.confirm(t('canvas.legacyHistoryConfirm')) || !session.isCurrent()) return
  importing.value = true
  error.value = ''
  try {
    await importLegacyStudioHistory(props.kind, session.owner, session.isCurrent)
    if (!session.isCurrent()) return
    available.value = false
    emit('imported')
  } catch {
    if (session.isCurrent()) error.value = t('canvas.legacyHistoryFailed')
  } finally {
    if (session.isCurrent()) importing.value = false
  }
}

function reset() {
  epoch++
  available.value = false
  importing.value = false
  error.value = ''
}

onMounted(() => {
  window.addEventListener('studio-session-ended', reset)
  void probe()
})
onUnmounted(() => {
  reset()
  window.removeEventListener('studio-session-ended', reset)
})
</script>
