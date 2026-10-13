<template>
  <dialog ref="dialog" class="quick-compose-dialog" :aria-labelledby="`${fieldId}-title`" @cancel.prevent="requestClose" @close="visible = false">
    <form v-if="visible" class="quick-compose-form" @submit.prevent="submit(true)">
      <header class="compose-heading">
        <div><p class="compose-eyebrow">{{ tr('eyebrow') }}</p><h2 :id="`${fieldId}-title`">{{ tr('title') }}</h2><p class="compose-intro">{{ tr('intro') }}</p></div>
        <button type="button" class="close-button" :aria-label="tr('close')" :disabled="busy" @click="requestClose">×</button>
      </header>

      <p v-if="prepared" class="compose-notice" role="status">{{ tr('preparedHint') }}</p>
      <p v-if="error" class="compose-error" role="alert">{{ error }}</p>
      <p v-if="validationError" class="compose-error" role="alert">{{ validationError }}</p>

      <fieldset v-if="!prepared" class="compose-fields" :disabled="locked">
        <div class="subject-fields">
          <label :for="`${fieldId}-subject`">{{ tr('subject') }}</label>
          <textarea :id="`${fieldId}-subject`" ref="subjectInput" v-model="draft.subject" rows="3" maxlength="2000" :placeholder="tr('subjectPlaceholder')" autofocus></textarea>
          <label :for="`${fieldId}-style`" class="optional-label">{{ tr('style') }} <span>{{ tr('optional') }}</span></label>
          <input :id="`${fieldId}-style`" v-model="draft.style" maxlength="1500" :placeholder="tr('stylePlaceholder')" />
        </div>

        <section class="compose-section" :aria-labelledby="`${fieldId}-images`">
          <div class="section-heading"><h3 :id="`${fieldId}-images`">{{ tr('images') }}</h3><span>{{ tr('oneOverview') }}</span></div>
          <div class="detail-options" role="group" :aria-label="tr('detailCount')">
            <button v-for="count in detailOptions" :key="count" type="button" :aria-pressed="draft.detailCount === count" @click="draft.detailCount = count"><strong>{{ count }}</strong><span>{{ tr('detailUnit') }}</span></button>
          </div>
          <p class="section-hint">{{ tr('detailHint') }}</p>
        </section>

        <section class="compose-section" :aria-labelledby="`${fieldId}-videos`">
          <div class="section-heading"><h3 :id="`${fieldId}-videos`">{{ tr('videos') }}</h3><span>{{ tr('videoHint') }}</span></div>
          <div class="video-options" role="group" :aria-label="tr('videoPlan')">
            <button v-for="mode in videoModes" :key="mode" type="button" :aria-pressed="draft.videoMode === mode" @click="draft.videoMode = mode"><strong>{{ tr(`video_${mode}`) }}</strong><span>{{ mode === 'none' ? tr('video_none_hint') : mode === 'two' ? tr('video_two_hint', { n: Math.min(2, counts.image) }) : tr('video_all_hint', { n: counts.image }) }}</span></button>
          </div>
        </section>

        <div class="compose-summary" aria-live="polite"><span class="summary-step"><strong>{{ counts.image }}</strong> {{ tr('imageUnit') }}</span><span v-if="counts.video" class="summary-arrow" aria-hidden="true">→</span><span v-if="counts.video" class="summary-step"><strong>{{ counts.video }}</strong> {{ tr('videoUnit') }}</span><span class="summary-total">{{ tr('steps', { n: counts.total }) }}</span></div>
        <label class="auto-timeline"><input v-model="draft.autoTimeline" type="checkbox" /> <span>{{ tr('autoTimeline') }}</span></label>

        <section class="compose-section models-section" :aria-labelledby="`${fieldId}-models`">
          <div class="section-heading"><h3 :id="`${fieldId}-models`">{{ tr('models') }}</h3><button type="button" class="advanced-toggle" :aria-expanded="advanced" :aria-controls="`${fieldId}-advanced`" @click="advanced = !advanced">{{ tr('advanced') }} <span aria-hidden="true">{{ advanced ? '−' : '+' }}</span></button></div>
          <div :id="`${fieldId}-advanced`" class="model-grid" :class="{ 'compact-fields': !advanced, 'images-only': !counts.video }">
            <div class="model-block"><h4>{{ tr('imageModel') }}</h4><StudioGenerationFields v-model="draft.imageParams" kind="image" :models="uniqueImageModels" :disabled="locked" /></div>
            <div v-if="counts.video" class="model-block"><h4>{{ tr('videoModel') }}</h4><StudioGenerationFields v-model="draft.videoParams" kind="video" :models="uniqueVideoModels" :disabled="locked" /></div>
            <div v-if="advanced" class="overview-field"><label :for="`${fieldId}-overview`">{{ tr('overview') }} <span>{{ tr('optional') }}</span></label><textarea :id="`${fieldId}-overview`" v-model="draft.overview" rows="2" maxlength="4000" :placeholder="tr('overviewPlaceholder')"></textarea></div>
          </div>
          <p v-if="!modelsReady" class="model-warning" role="status">{{ tr('missingModels') }}</p>
        </section>
      </fieldset>

      <footer class="compose-footer">
        <p>{{ prepared ? tr('continueHint') : tr('billingHint') }}</p>
        <div class="footer-actions">
          <button v-if="!prepared" type="button" class="draft-button" :disabled="busy || !hasSubject || !!validationError" @click="submit(false)">{{ tr('createOnly') }}</button>
          <button v-else type="button" class="draft-button" :disabled="busy" @click="requestClose">{{ tr('backToCanvas') }}</button>
          <button type="submit" class="generate-button" :disabled="busy || !hasSubject || !modelsReady || !!validationError"><span v-if="busy" class="button-spinner" aria-hidden="true"></span>{{ busy ? tr('working') : prepared ? tr('continue') : tr('generate') }}</button>
        </div>
      </footer>
    </form>
  </dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { StudioModel, StudioParams, StudioSettings } from '@/api/creativeStudio'
import { quickComposeCounts, type QuickComposeInput } from '@/types/creativeQuickCompose'
import StudioGenerationFields from './StudioGenerationFields.vue'

const props = withDefaults(defineProps<{
  imageModels: StudioModel[]
  videoModels: StudioModel[]
  defaults?: StudioSettings
  busy?: boolean
  error?: string
  prepared?: boolean
}>(), { busy: false, error: '', prepared: false })
const emit = defineEmits<{ submit: [input: QuickComposeInput, generate: boolean] }>()
const { t: tr } = useI18n({ useScope: 'local', messages: {
  zh: {
    eyebrow: '快捷创作', title: '一个想法，生成一整套', intro: '描述一次主体，自动连接整体图、细节图与视频镜头。', close: '关闭',
    subject: '想展示什么？', subjectPlaceholder: '例如：焦糖棕色皮革手提包，金色锁扣，弧形手柄，保留皮革纹理和缝线细节。',
    style: '视觉风格', optional: '选填', stylePlaceholder: '例如：白色背景、柔和自然光、写实产品摄影',
    images: '图片组合', oneOverview: '固定 1 张整体图', detailCount: '细节图数量', detailUnit: '张细节',
    detailHint: '细节图会参考同一张整体图，保持主体、颜色与材质一致。',
    videos: '让图片动起来', videoHint: '图片完成后自动继续', videoPlan: '视频生成方案',
    video_none: '只要图片', video_none_hint: '之后也可以添加视频', video_two: '精选镜头', video_two_hint: '生成 {n} 段视频', video_all: '每张都做视频', video_all_hint: '生成 {n} 段视频',
    imageUnit: '张图片', videoUnit: '段视频', steps: '共 {n} 个生成步骤',
    autoTimeline: '生成完成后自动整理到时间线',
    models: '使用模型', advanced: '高级参数', imageModel: '图片', videoModel: '视频',
    overview: '整体画面与构图', overviewPlaceholder: '例如：完整展示主体，三分之四视角，主体居中，画面留白。',
    missingModels: '部分模型不可用，请检查工作台分组权限或重新选择模型。仍可先创建流程。',
    fieldTooLong: '“{field}”内容过长，请缩短后再创建流程。',
    billingHint: '将创建独立项目。“只创建流程”不调用模型；生成按实际模型用量扣费。',
    createOnly: '只创建流程', generate: '创建并生成', working: '处理中…', continue: '继续生成此流程',
    preparedHint: '流程已创建。继续会使用画布中当前保存的描述、模型和参数；也可以返回画布调整后再继续。',
    backToCanvas: '返回画布调整', continueHint: '继续提交同一套流程；生成按实际模型用量扣费。'
  },
  en: {
    eyebrow: 'Quick compose', title: 'One idea. A complete set.', intro: 'Describe your subject once. Overview, details and video shots are connected for you.', close: 'Close',
    subject: 'What would you like to show?', subjectPlaceholder: 'For example: a caramel leather handbag, a gold clasp and curved handles. Keep the leather texture and stitching.',
    style: 'Visual style', optional: 'Optional', stylePlaceholder: 'For example: white background, soft daylight, realistic product photography',
    images: 'Image set', oneOverview: 'Includes 1 overview', detailCount: 'Number of detail images', detailUnit: 'details',
    detailHint: 'Details use the same overview as a reference to preserve the subject, colors and materials.',
    videos: 'Bring images to life', videoHint: 'Starts when the images are ready', videoPlan: 'Video plan',
    video_none: 'Images only', video_none_hint: 'You can add video later', video_two: 'Selected shots', video_two_hint: 'Generate {n} videos', video_all: 'Animate every image', video_all_hint: 'Generate {n} videos',
    imageUnit: 'images', videoUnit: 'videos', steps: '{n} generation steps',
    autoTimeline: 'Add completed results to the timeline automatically',
    models: 'Models', advanced: 'Advanced settings', imageModel: 'Images', videoModel: 'Video',
    overview: 'Overview framing', overviewPlaceholder: 'For example: show the whole subject, three-quarter view, centered with room around it.',
    missingModels: 'Some models are unavailable. Check studio group access or select another model. You can still create the workflow.',
    fieldTooLong: '{field} is too long. Shorten it before creating the workflow.',
    billingHint: 'Creates a separate project. Creating the workflow does not call a model. Generation is billed for actual model usage.',
    createOnly: 'Create workflow only', generate: 'Create and generate', working: 'Working…', continue: 'Continue this workflow',
    preparedHint: 'Your workflow is ready. Continue with the current descriptions, models and settings on the canvas, or return to the canvas to adjust them first.',
    backToCanvas: 'Adjust on canvas', continueHint: 'Continue submitting the same workflow. Generation is billed for actual model usage.'
  }
} })

const dialog = ref<HTMLDialogElement>()
const subjectInput = ref<HTMLTextAreaElement>()
const visible = ref(false)
const advanced = ref(false)
const fieldId = `quick-compose-${Math.random().toString(36).slice(2)}`
const detailOptions = [0, 1, 2, 3, 4]
const videoModes: QuickComposeInput['videoMode'][] = ['none', 'two', 'all']
const uniqueModels = (models: StudioModel[]) => [...new Map(models.filter(model => model.group_id > 0).map(model => [JSON.stringify([model.group_id, model.id]), model])).values()]
const uniqueImageModels = computed(() => uniqueModels(props.imageModels))
const uniqueVideoModels = computed(() => uniqueModels(props.videoModels))
const baseParams = (): StudioParams => ({ model: '', group_id: 0, size: 'auto', quality: 'auto', format: 'png', seconds: 6, resolution: '720p' })
const draft = ref<QuickComposeInput>({ subject: '', style: '', overview: '', detailCount: 4, videoMode: 'two', autoTimeline: true, imageParams: baseParams(), videoParams: baseParams() })
const locked = computed(() => props.busy || props.prepared)
const counts = computed(() => quickComposeCounts(draft.value.detailCount, draft.value.videoMode))
const hasSubject = computed(() => props.prepared || !!draft.value.subject.trim())
const available = (params: StudioParams, models: StudioModel[]) => models.some(model => model.group_id > 0 && model.group_id === params.group_id && model.id === params.model)
const modelsReady = computed(() => props.prepared || (available(draft.value.imageParams, props.imageModels) && (!counts.value.video || available(draft.value.videoParams, props.videoModels))))
const validationError = computed(() => {
  if (props.prepared) return ''
  const encoder = new TextEncoder()
  for (const [field, limit] of [['subject', 4000], ['style', 4000], ['overview', 12000]] as const) {
    if (encoder.encode(draft.value[field].trim()).byteLength > limit) return tr('fieldTooLong', { field: tr(field) })
  }
  return ''
})

function initialParams(value: StudioParams | undefined, models: StudioModel[]): StudioParams {
  if (value && available(value, models)) return { ...value }
  const model = models.find(item => item.group_id > 0)
  if (!model) return value ? { ...value } : baseParams()
  return { ...baseParams(), model: model.id, group_id: model.group_id, platform: model.platform }
}
async function open() {
  if (props.busy) return
  if (!props.prepared) {
    draft.value = {
      subject: props.defaults?.subject || '', style: props.defaults?.style || '', overview: '', detailCount: 4, videoMode: 'two', autoTimeline: true,
      imageParams: initialParams(props.defaults?.image_params, props.imageModels), videoParams: initialParams(props.defaults?.video_params, props.videoModels)
    }
    advanced.value = false
  }
  visible.value = true
  await nextTick()
  if (!dialog.value?.open) dialog.value?.showModal()
  if (!props.prepared) subjectInput.value?.focus()
}
function close() {
  dialog.value?.close()
  visible.value = false
}
function requestClose() { if (!props.busy) close() }
function submit(generate: boolean) {
  if (props.busy || !hasSubject.value || validationError.value || (generate && !modelsReady.value) || (props.prepared && !generate)) return
  emit('submit', { ...draft.value, subject: draft.value.subject.trim(), style: draft.value.style.trim(), overview: draft.value.overview.trim(), imageParams: { ...draft.value.imageParams }, videoParams: { ...draft.value.videoParams } }, generate)
}
defineExpose({ open, close })
</script>

<style scoped>
.quick-compose-dialog { --cs-bg: #fff; --cs-surface: #f8fafc; --cs-border: #e2e8f0; --cs-text: #334155; --cs-muted: #64748b; width: min(690px, calc(100vw - 32px)); max-height: min(92dvh, 1000px); margin: auto; padding: 0; overflow-y: auto; border: 1px solid var(--cs-border); border-radius: 18px; background: var(--cs-bg); color: var(--cs-text); box-shadow: 0 24px 80px #0f172a35; }
.quick-compose-dialog::backdrop { background: #0f172a66; backdrop-filter: blur(3px); }
.quick-compose-form { padding: 28px 30px 0; }
.compose-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; margin-bottom: 22px; }
.compose-eyebrow { margin: 0 0 6px; font-size: 11px; font-weight: 600; letter-spacing: .1em; color: #0f766e; }
.compose-heading h2 { margin: 0; color: #0f172a; font-size: 24px; font-weight: 650; line-height: 1.4; letter-spacing: -.03em; }
.compose-intro { margin: 8px 0 0; color: var(--cs-muted); font-size: 12px; line-height: 1.7; }
.quick-compose-dialog button { font: inherit; cursor: pointer; }
.close-button { display: grid; place-items: center; flex-shrink: 0; width: 30px; height: 30px; padding: 0; border: 0; border-radius: 7px; background: var(--cs-surface); color: var(--cs-muted); font-size: 23px !important; line-height: 1; }
.compose-fields { min-width: 0; padding: 0; margin: 0; border: 0; }
.subject-fields label,.overview-field label { display: block; margin-bottom: 7px; font-size: 12px; font-weight: 600; }
.subject-fields textarea,.subject-fields input,.overview-field textarea { box-sizing: border-box; width: 100%; border: 1px solid var(--cs-border); border-radius: 9px; padding: 10px 12px; background: var(--cs-bg); color: var(--cs-text); font: inherit; font-size: 13px; line-height: 1.6; }
.subject-fields textarea,.overview-field textarea { display: block; resize: vertical; min-height: 76px; }
.subject-fields .optional-label { margin-top: 15px; }
.optional-label span,.overview-field label span { margin-left: 5px; color: var(--cs-muted); font-size: 10px; font-weight: 400; }
.quick-compose-dialog :is(button,input,select,textarea):focus-visible { outline: 2px solid #14b8a6; outline-offset: 2px; }
.compose-section { margin-top: 22px; }
.section-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
.section-heading h3 { margin: 0; color: var(--cs-text); font-size: 12px; font-weight: 600; }
.section-heading > span { color: var(--cs-muted); font-size: 10px; }
.detail-options { display: grid; grid-template-columns: repeat(5,minmax(0,1fr)); gap: 8px; }
.detail-options button,.video-options button { display: flex; align-items: center; justify-content: center; gap: 5px; min-width: 0; padding: 10px 7px; border: 1px solid var(--cs-border); border-radius: 9px; background: var(--cs-bg); color: var(--cs-muted); transition: background .15s, border-color .15s; }
.detail-options button strong { color: var(--cs-text); font-size: 16px; font-weight: 600; }
.detail-options button span { font-size: 10px; }
.detail-options button[aria-pressed=true],.video-options button[aria-pressed=true] { border-color: #14b8a6; background: #f0fdfa; color: #0f766e; }
.detail-options button[aria-pressed=true] strong,.video-options button[aria-pressed=true] strong { color: #0f766e; }
.section-hint { margin: 8px 0 0; color: var(--cs-muted); font-size: 10px; line-height: 1.7; }
.video-options { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 8px; }
.video-options button { flex-direction: column; align-items: flex-start; gap: 5px; padding: 12px; text-align: left; }
.video-options strong { color: var(--cs-text); font-size: 12px; font-weight: 600; }
.video-options span { font-size: 10px; line-height: 1.5; }
.compose-summary { display: flex; align-items: center; flex-wrap: wrap; gap: 11px; margin-top: 18px; padding: 13px 15px; border-radius: 9px; background: var(--cs-surface); font-size: 11px; }
.auto-timeline { display:flex;align-items:center;gap:7px;margin-top:12px;font-size:11px;color:var(--cs-muted);cursor:pointer; }
.auto-timeline input { width:14px;height:14px;accent-color:#0f766e; }
.summary-step strong { margin-right: 2px; font-size: 17px; color: #0f766e; }
.summary-arrow { color: #94a3b8; }
.summary-total { margin-left: auto; color: var(--cs-muted); font-size: 10px; }
.models-section { padding-top: 18px; border-top: 1px solid var(--cs-border); }
.advanced-toggle { border: 0; padding: 2px 0; background: transparent; color: #0f766e; font-size: 11px !important; }
.advanced-toggle span { display: inline-block; width: 12px; margin-left: 3px; }
.model-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 14px; }
.model-grid.images-only { grid-template-columns: minmax(0,1fr); }
.model-block { min-width: 0; }
.model-block h4 { margin: 3px 0 8px; color: var(--cs-muted); font-size: 10px; font-weight: 500; }
.compact-fields :deep(.field-grid),.compact-fields :deep(.muted:not([role=status])) { display: none; }
.model-block :deep(.generation-fields > label) { margin-bottom: 7px; }
.overview-field { grid-column: 1 / -1; }
.model-warning,.compose-notice,.compose-error { padding: 10px 12px; border-radius: 8px; font-size: 11px; line-height: 1.7; }
.model-warning { margin: 8px 0 0; color: #92400e; background: #fffbeb; }
.compose-notice { margin: 0 0 18px; color: #0f766e; background: #f0fdfa; }
.compose-error { margin: 0 0 18px; color: #b91c1c; background: #fef2f2; overflow-wrap: anywhere; }
.compose-footer { position: sticky; bottom: 0; margin: 22px -30px 0; padding: 14px 30px 20px; border-top: 1px solid var(--cs-border); background: var(--cs-bg); }
.compose-footer > p { margin: 0 0 12px; color: var(--cs-muted); font-size: 10px; line-height: 1.65; }
.footer-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 9px; }
.footer-actions button { display: inline-flex; justify-content: center; align-items: center; gap: 7px; min-height: 39px; border-radius: 8px; padding: 8px 17px; font-size: 12px; font-weight: 600; }
.draft-button { border: 1px solid var(--cs-border); background: var(--cs-bg); color: var(--cs-text); }
.generate-button { border: 1px solid #0f766e; background: #0f766e; color: white; }
.quick-compose-dialog :disabled { cursor: not-allowed; opacity: .58; }
.button-spinner { width: 12px; height: 12px; border: 2px solid #ffffff60; border-top-color: white; border-radius: 50%; animation: compose-spin .8s linear infinite; }
@keyframes compose-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .button-spinner { animation: none; } }
:global(.dark) .quick-compose-dialog { --cs-bg: #17202e; --cs-surface: #1e293b; --cs-border: #334155; --cs-text: #e2e8f0; --cs-muted: #94a3b8; }
:global(.dark) .compose-heading h2 { color: #f8fafc; }
:global(.dark) :is(.compose-eyebrow,.advanced-toggle,.summary-step strong) { color: #5eead4; }
:global(.dark) .detail-options button[aria-pressed=true],:global(.dark) .video-options button[aria-pressed=true],:global(.dark) .compose-notice { background: #103735; color: #5eead4; }
:global(.dark) .detail-options button[aria-pressed=true] strong,:global(.dark) .video-options button[aria-pressed=true] strong { color: #5eead4; }
:global(.dark) .model-warning { background: #422d15; color: #fcd34d; }
:global(.dark) .compose-error { background: #451a20; color: #fda4af; }
@media (max-width: 560px) { .quick-compose-dialog { width: calc(100vw - 20px); max-height: 94dvh; border-radius: 13px; } .quick-compose-form { padding: 20px 17px 0; } .compose-heading h2 { font-size: 21px; } .compose-footer { margin-left: -17px; margin-right: -17px; padding: 13px 17px 16px; } .model-grid { grid-template-columns: minmax(0,1fr); gap: 6px; } .detail-options { gap: 5px; } .detail-options button { flex-direction: column; gap: 2px; } .video-options { gap: 6px; } .video-options button { padding: 10px 8px; } .video-options strong { font-size: 11px; } .section-heading > span { max-width: 55%; text-align: right; line-height: 1.5; } .footer-actions { flex-wrap: nowrap; } .footer-actions button { flex: 1; padding: 8px 10px; } }
</style>
