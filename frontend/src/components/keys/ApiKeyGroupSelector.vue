<template>
  <div class="space-y-3" data-test="api-key-group-selector">
    <div
      v-if="selectedEntries.length > 0"
      class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600"
    >
      <div
        v-for="(entry, index) in selectedEntries"
        :key="entry.id"
        class="flex min-h-12 items-center gap-2 border-b border-gray-100 px-3 py-2 last:border-b-0 dark:border-dark-700"
        :data-test="`selected-group-${entry.id}`"
      >
        <span
          class="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-gray-100 text-xs font-semibold tabular-nums text-gray-600 dark:bg-dark-700 dark:text-gray-300"
          :title="t('keys.groupPriority', { priority: index + 1 })"
        >
          {{ index + 1 }}
        </span>

        <div class="min-w-0 flex-1">
          <GroupBadge
            v-if="entry.group"
            :name="entry.group.name"
            :platform="entry.group.platform"
            :subscription-type="entry.group.subscription_type"
            :rate-multiplier="entry.group.rate_multiplier"
            :user-rate-multiplier="userGroupRates[entry.group.id]"
            :peak-rate-enabled="entry.group.peak_rate_enabled"
            :peak-start="entry.group.peak_start"
            :peak-end="entry.group.peak_end"
            :peak-rate-multiplier="entry.group.peak_rate_multiplier"
          />
          <span v-else class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('keys.groupFallback', { id: entry.id }) }}
          </span>
        </div>

        <div class="flex shrink-0 items-center gap-1">
          <button
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 disabled:cursor-not-allowed disabled:opacity-30 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
            :disabled="index === 0"
            :title="t('keys.moveGroupUp')"
            :aria-label="t('keys.moveGroupUp')"
            :data-test="`move-group-up-${entry.id}`"
            @click="moveGroup(index, -1)"
          >
            <Icon name="chevronUp" size="sm" />
          </button>
          <button
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 disabled:cursor-not-allowed disabled:opacity-30 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
            :disabled="index === selectedEntries.length - 1"
            :title="t('keys.moveGroupDown')"
            :aria-label="t('keys.moveGroupDown')"
            :data-test="`move-group-down-${entry.id}`"
            @click="moveGroup(index, 1)"
          >
            <Icon name="chevronDown" size="sm" />
          </button>
          <button
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:text-gray-400 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            :title="t('keys.removeGroup')"
            :aria-label="t('keys.removeGroup')"
            :data-test="`remove-group-${entry.id}`"
            @click="removeGroup(index)"
          >
            <Icon name="x" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <div
      v-else
      class="rounded-lg border border-dashed border-gray-300 px-3 py-4 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
      data-test="empty-groups"
    >
      {{ t('keys.noGroupsSelected') }}
    </div>

    <Select
      :model-value="null"
      :options="addableOptions"
      :placeholder="addPlaceholder"
      :searchable="true"
      :search-placeholder="t('keys.searchGroup')"
      :empty-text="t('keys.noCompatibleGroups')"
      :disabled="modelValue.length >= maxGroups"
      data-test="add-group-select"
      @update:model-value="addGroup"
    >
      <template #selected>
        <span class="text-gray-500 dark:text-gray-400">{{ addPlaceholder }}</span>
      </template>
      <template #option="{ option }">
        <GroupOptionItem
          :name="(option as unknown as GroupOption).label"
          :platform="(option as unknown as GroupOption).group.platform"
          :subscription-type="(option as unknown as GroupOption).group.subscription_type"
          :rate-multiplier="(option as unknown as GroupOption).group.rate_multiplier"
          :user-rate-multiplier="userGroupRates[(option as unknown as GroupOption).group.id]"
          :peak-rate-enabled="(option as unknown as GroupOption).group.peak_rate_enabled"
          :peak-start="(option as unknown as GroupOption).group.peak_start"
          :peak-end="(option as unknown as GroupOption).group.peak_end"
          :peak-rate-multiplier="(option as unknown as GroupOption).group.peak_rate_multiplier"
          :description="(option as unknown as GroupOption).group.description"
          :show-checkmark="false"
        />
      </template>
    </Select>

    <div class="flex items-start justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
      <span>{{ routingHint }}</span>
      <span class="shrink-0 tabular-nums" data-test="group-count">
        {{ t('keys.selectedGroupCount', { count: modelValue.length, max: maxGroups }) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Group } from '@/types'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  API_KEY_GROUP_LIMIT,
  moveApiKeyGroup,
  normalizeApiKeyGroupIds
} from '@/utils/apiKeyGroups'

interface GroupOption extends Record<string, unknown> {
  value: number
  label: string
  description: string | null
  group: Group
}

interface Props {
  modelValue: number[]
  groups: Group[]
  userGroupRates?: Record<number, number>
  maxGroups?: number
}

const props = withDefaults(defineProps<Props>(), {
  userGroupRates: () => ({}),
  maxGroups: API_KEY_GROUP_LIMIT
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: number[]): void
}>()

const { t } = useI18n()

const groupsById = computed(() => new Map(props.groups.map((group) => [group.id, group])))
const selectedEntries = computed(() =>
  props.modelValue.map((id) => ({ id, group: groupsById.value.get(id) }))
)
const selectedPlatform = computed(() => selectedEntries.value.find((entry) => entry.group)?.group?.platform)

const addableOptions = computed<GroupOption[]>(() => {
  const selectedIds = new Set(props.modelValue)
  return props.groups
    .filter((group) => !selectedIds.has(group.id))
    .filter((group) => !selectedPlatform.value || group.platform === selectedPlatform.value)
    .filter(() => selectedPlatform.value !== 'composite')
    .map((group) => ({
      value: group.id,
      label: group.name,
      description: group.description,
      group
    }))
})

const addPlaceholder = computed(() =>
  props.modelValue.length >= props.maxGroups
    ? t('keys.groupLimitReached', { max: props.maxGroups })
    : t('keys.addFailoverGroup')
)

const routingHint = computed(() =>
  selectedPlatform.value === 'composite'
    ? t('keys.compositeSingleGroupHint')
    : t('keys.samePlatformGroupsHint')
)

const updateGroups = (ids: number[]) => {
  emit('update:modelValue', normalizeApiKeyGroupIds(ids, props.groups).slice(0, props.maxGroups))
}

const addGroup = (value: string | number | boolean | null) => {
  if (typeof value !== 'number' || props.modelValue.length >= props.maxGroups) return
  updateGroups([...props.modelValue, value])
}

const removeGroup = (index: number) => {
  updateGroups(props.modelValue.filter((_, currentIndex) => currentIndex !== index))
}

const moveGroup = (index: number, direction: -1 | 1) => {
  updateGroups(moveApiKeyGroup(props.modelValue, index, direction))
}

watch(
  () => [props.modelValue, props.groups] as const,
  () => {
    const normalized = normalizeApiKeyGroupIds(props.modelValue, props.groups).slice(0, props.maxGroups)
    if (
      normalized.length !== props.modelValue.length ||
      normalized.some((id, index) => id !== props.modelValue[index])
    ) {
      emit('update:modelValue', normalized)
    }
  },
  { deep: true, immediate: true }
)
</script>
