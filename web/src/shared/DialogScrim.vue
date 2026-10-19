<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

/** Dims the page behind a dialog; Esc or a click beside the dialog closes it, and its first field takes the cursor. */
const emit = defineEmits<{ close: [] }>()
const scrim = ref<HTMLElement | null>(null)
function esc(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
function beside(e: MouseEvent) {
  if (e.target === scrim.value) emit('close')
}
onMounted(() => {
  document.addEventListener('keydown', esc)
  scrim.value?.addEventListener('click', beside)
  scrim.value?.querySelector<HTMLElement>('input, button')?.focus()
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', esc)
})
</script>

<template>
  <div ref="scrim" class="scrim"><slot /></div>
</template>
