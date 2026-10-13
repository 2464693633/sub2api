<template>
  <div class="generation-fields">
    <label>{{ tr('model') }}
      <select :value="modelKey(modelValue)" :disabled="disabled" @change="chooseModel(($event.target as HTMLSelectElement).value)">
        <option v-if="!models.some(m => modelKey(m) === modelKey(modelValue))" :value="modelKey(modelValue)">{{ modelValue.model || tr('noModel') }}</option>
        <option v-for="m in models" :key="modelKey(m)" :value="modelKey(m)">{{ m.id }} · {{ m.group_name }}</option>
      </select>
    </label>
    <p v-if="!models.length" class="warning">{{ tr('noModel') }}</p>
    <p v-if="loading" class="muted" role="status">{{ tr('checkingCapabilities') }}</p>
    <p v-if="loadError" class="warning">{{ tr('capabilitiesUnavailable') }}</p>
    <div v-if="capability" class="field-grid">
      <label v-if="capability.sizes.length || capability.custom_size">{{ tr('size') }}
        <input v-if="capability.custom_size" :value="modelValue.size" :disabled="disabled" :list="`${fieldId}-sizes`" @change="set('size', ($event.target as HTMLInputElement).value)" />
        <select v-else :value="modelValue.size" :disabled="disabled" @change="set('size', ($event.target as HTMLSelectElement).value)"><option v-for="v in values(capability.sizes, modelValue.size)" :key="v" :value="v">{{ v === 'auto' ? tr('auto') : v }}</option></select>
        <datalist :id="`${fieldId}-sizes`"><option v-for="v in capability.sizes" :key="v" :value="v" /></datalist>
      </label>
      <label v-if="kind === 'image' && (capability.qualities.length || capability.custom_quality)">{{ tr('quality') }}<input v-if="capability.custom_quality" :value="modelValue.quality" :disabled="disabled" @change="set('quality', ($event.target as HTMLInputElement).value)" /><select v-else :value="modelValue.quality" :disabled="disabled" @change="set('quality', ($event.target as HTMLSelectElement).value)"><option v-for="v in values(capability.qualities, modelValue.quality)" :key="v" :value="v">{{ ['auto','high','medium','low'].includes(v) ? tr(v) : v }}</option></select></label>
      <label v-if="kind === 'image' && (capability.formats.length || capability.custom_format)">{{ tr('format') }}<input v-if="capability.custom_format" :value="modelValue.format" :disabled="disabled" @change="set('format', ($event.target as HTMLInputElement).value)" /><select v-else :value="modelValue.format" :disabled="disabled" @change="set('format', ($event.target as HTMLSelectElement).value)"><option v-for="v in values(capability.formats, modelValue.format)" :key="v">{{ v }}</option></select></label>
      <label v-if="kind === 'video'">{{ tr('seconds') }}
        <input v-if="capability.custom_seconds" :value="modelValue.seconds" type="number" min="1" :max="capability.max_seconds || 30" step="1" :disabled="disabled" @change="set('seconds', Number(($event.target as HTMLInputElement).value))" />
        <select v-else :value="modelValue.seconds" :disabled="disabled" @change="set('seconds', Number(($event.target as HTMLSelectElement).value))"><option v-for="v in values(capability.seconds, modelValue.seconds)" :key="v" :value="v">{{ v }}</option></select>
      </label>
      <label v-if="kind === 'video' && (capability.resolutions.length || capability.custom_resolution)">{{ tr('resolution') }}
        <input v-if="capability.custom_resolution" :value="modelValue.resolution" :disabled="disabled" :list="`${fieldId}-resolutions`" @change="set('resolution', ($event.target as HTMLInputElement).value)" />
        <select v-else :value="modelValue.resolution" :disabled="disabled" @change="set('resolution', ($event.target as HTMLSelectElement).value)"><option v-for="v in values(capability.resolutions, modelValue.resolution)" :key="v">{{ v }}</option></select>
        <datalist :id="`${fieldId}-resolutions`"><option v-for="v in capability.resolutions" :key="v" :value="v" /></datalist>
      </label>
    </div>
    <p v-if="capability" class="muted">{{ tr('referenceLimit', { n: capability.max_references }) }}</p>
    <p v-for="note in capability?.notes || []" :key="note" class="muted">{{ note }}</p>
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { creativeStudioAPI, type StudioParams, type StudioModel, type StudioCapabilities } from '@/api/creativeStudio'
import { creativeStudioMessages } from './creativeStudioMessages'
const props = withDefaults(defineProps<{ modelValue: StudioParams; kind: 'image' | 'video'; models: StudioModel[]; disabled?: boolean }>(), { disabled: false })
const emit = defineEmits<{ 'update:modelValue': [value: StudioParams] }>()
const { t: tr } = useI18n({ useScope: 'local', messages: creativeStudioMessages })
const fieldId = `generation-${Math.random().toString(36).slice(2)}`
const capability = ref<StudioCapabilities>()
const loading = ref(false)
const loadError = ref(false)
let controller: AbortController | undefined
let normalizeNext = false
const modelKey = (m: Pick<StudioModel, 'id' | 'group_id'> | StudioParams) => JSON.stringify([m.group_id, 'id' in m ? m.id : m.model])
const values = <T extends string | number>(available: T[], current: T) => [...new Set([...available, ...(current !== '' && !available.includes(current) ? [current] : [])])]
function set<K extends keyof StudioParams>(key: K, value: StudioParams[K]) { emit('update:modelValue', { ...props.modelValue, [key]: value }) }
function chooseModel(key: string) {
  const model = props.models.find(m => modelKey(m) === key)
  if (!model) return
  normalizeNext = true
  emit('update:modelValue', { ...props.modelValue, model: model.id, group_id: model.group_id, platform: model.platform })
}
watch(() => [props.kind, props.modelValue.model, props.modelValue.platform], async () => {
  controller?.abort()
  controller = new AbortController()
  const request = controller
  const normalize = normalizeNext
  normalizeNext = false
  capability.value = undefined; loadError.value = false
  if (!props.modelValue.model) { loading.value = false; return }
  loading.value = true
  try {
    const result = await creativeStudioAPI.capabilities(props.kind, props.modelValue.model, props.modelValue.platform, request.signal)
    if (request.signal.aborted) return
    capability.value = result
    if (normalize) {
      const next = { ...props.modelValue }
      if (!result.custom_size && result.sizes.length && !result.sizes.includes(next.size)) next.size = result.sizes[0]!
      if (!result.custom_quality && result.qualities.length && !result.qualities.includes(next.quality)) next.quality = result.qualities[0]!
      if (!result.custom_format && result.formats.length && !result.formats.includes(next.format)) next.format = result.formats[0]!
      if (!result.custom_seconds && result.seconds.length && !result.seconds.includes(next.seconds)) next.seconds = result.seconds[0]!
      if (!result.custom_resolution && result.resolutions.length && !result.resolutions.includes(next.resolution)) next.resolution = result.resolutions[0]!
      emit('update:modelValue', next)
    }
  } catch { if (!request.signal.aborted) loadError.value = true }
  finally { if (!request.signal.aborted) loading.value = false }
}, { immediate: true })
onBeforeUnmount(() => controller?.abort())
</script>
<style scoped>
.generation-fields label { display:flex;flex-direction:column;gap:6px;margin-bottom:12px;color:var(--cs-muted);font-size:11px; }
.generation-fields input,.generation-fields select { width:100%;min-width:0;border:1px solid var(--cs-border);border-radius:6px;padding:7px 8px;background:var(--cs-bg);color:var(--cs-text);font-size:12px; }
.field-grid { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px; }
.muted { color:var(--cs-muted);font-size:11px;line-height:1.6;margin:5px 0; }
.warning { color:#b45309;font-size:11px; }
:disabled { opacity:.65; }
</style>
