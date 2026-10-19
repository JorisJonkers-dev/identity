<script setup lang="ts" generic="T extends string">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from './Icon'
import type { IconName } from './icons'

/** A dropdown as the design draws it: a glass button that opens a glass menu. */
const props = defineProps<{
  options: { value: T; label: string }[]
  label: string
  icon?: IconName
  placeholder?: string
  small?: boolean
}>()
const model = defineModel<string>({ default: '' })
const emit = defineEmits<{ pick: [value: T] }>()
const open = ref(false)
const root = ref<HTMLElement | null>(null)
const shown = computed(() => props.options.find((o) => o.value === model.value)?.label ?? props.placeholder ?? '')

function choose(value: T) {
  if (!props.placeholder) model.value = value
  emit('pick', value)
  open.value = false
}
function outside(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) open.value = false
}
function esc(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}
onMounted(() => {
  document.addEventListener('click', outside)
  document.addEventListener('keydown', esc)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', outside)
  document.removeEventListener('keydown', esc)
})
</script>

<template>
  <div ref="root" :class="['dd', { open, sm: small }]">
    <button class="dd-btn" type="button" aria-haspopup="listbox" :aria-expanded="open" :aria-label="label" @click="open = !open">
      <Icon v-if="icon" :name="icon" />
      <span class="dd-l">{{ shown }}</span>
      <Icon name="chev" class="dd-chev" />
    </button>
    <div class="dd-menu glass strong" role="listbox" :aria-label="label">
      <button
        v-for="o in options"
        :key="o.value"
        type="button"
        role="option"
        :aria-selected="!placeholder && o.value === model"
        @click="choose(o.value)"
      >
        {{ o.label }}
      </button>
    </div>
  </div>
</template>
