<template>
  <div class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
    <div class="flex items-center justify-between gap-3">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.payment.rechargeTiers') }}</h4>
      <button type="button" class="btn btn-secondary text-sm" :disabled="modelValue.length >= 50" @click="addTier">
        {{ t('admin.settings.payment.addRechargeTier') }}
      </button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.rechargeTiersHint') }}</p>
    <p v-if="modelValue.length === 0" class="text-sm text-gray-400">{{ t('admin.settings.payment.noRechargeTiers') }}</p>
    <div v-for="(tier, index) in modelValue" :key="index" class="grid grid-cols-1 items-end gap-3 rounded-lg bg-gray-50 p-3 sm:grid-cols-[1fr_1fr_1fr_auto] dark:bg-dark-800">
      <div>
        <label :for="`tier-currency-${index}`" class="input-label">{{ t('admin.settings.payment.tierCurrency') }}</label>
        <select :id="`tier-currency-${index}`" class="input" :value="tier.currency" @change="updateTier(index, 'currency', ($event.target as HTMLSelectElement).value)">
          <option v-for="currency in currencies" :key="currency" :value="currency">{{ currency }}</option>
        </select>
      </div>
      <div>
        <label :for="`tier-amount-${index}`" class="input-label">{{ t('admin.settings.payment.tierAmount') }}</label>
        <input :id="`tier-amount-${index}`" type="number" :min="amountStep(tier.currency)" max="1000000000" :step="amountStep(tier.currency)" class="input" :value="tier.amount || ''" placeholder="100" @input="updateTier(index, 'amount', Number(($event.target as HTMLInputElement).value))" />
      </div>
      <div>
        <label :for="`tier-credit-${index}`" class="input-label">{{ t('admin.settings.payment.tierCredit') }}</label>
        <input :id="`tier-credit-${index}`" type="number" min="0.01" max="1000000000" step="0.01" class="input" :value="tier.credit_amount || ''" placeholder="1100" @input="updateTier(index, 'credit_amount', Number(($event.target as HTMLInputElement).value))" />
      </div>
      <button type="button" class="btn btn-secondary text-red-600" :aria-label="t('admin.settings.payment.removeRechargeTier')" @click="emit('update:modelValue', modelValue.filter((_, row) => row !== index))">
        {{ t('common.delete') }}
      </button>
      <p class="text-xs font-medium text-primary-600 sm:col-span-4 dark:text-primary-400">
        {{ t('admin.settings.payment.tierPreview', { pay: formatPaymentAmount(tier.amount, tier.currency), credit: tier.credit_amount.toFixed(2) }) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { RechargeTier } from '@/types/payment'

const props = withDefaults(defineProps<{ modelValue?: RechargeTier[] }>(), { modelValue: () => [] })
const emit = defineEmits<{ 'update:modelValue': [value: RechargeTier[]] }>()
const { t } = useI18n()
const currencies = computed(() => [...new Set(['CNY', 'USD', 'HKD', 'EUR', 'GBP', 'JPY', 'AUD', 'CAD', 'SGD', ...props.modelValue.map(tier => tier.currency)])])

function amountStep(currency: string): number {
  try {
    return 10 ** -Math.min(2, new Intl.NumberFormat(undefined, { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2)
  } catch {
    return 0.01
  }
}

function addTier() {
  if (props.modelValue.length >= 50) return
  const tier: RechargeTier = props.modelValue.length === 0
    ? { amount: 100, credit_amount: 1100, currency: 'CNY' }
    : { amount: 0, credit_amount: 0, currency: props.modelValue[props.modelValue.length - 1].currency }
  emit('update:modelValue', [...props.modelValue, tier])
}

function updateTier(index: number, field: keyof RechargeTier, value: string | number) {
  emit('update:modelValue', props.modelValue.map((tier, row) => row === index ? { ...tier, [field]: value } : tier))
}
</script>
