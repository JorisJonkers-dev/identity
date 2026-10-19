<script setup lang="ts">
import { computed, ref } from 'vue'

/** Six boxes for a six-digit code: typing moves on, backspace moves back, a paste fills them all. */
defineProps<{ invalid?: boolean; disabled?: boolean }>()
const code = defineModel<string>({ default: '' })
const boxes = ref<HTMLInputElement[]>([])
const digits = computed(() => Array.from({ length: 6 }, (_, i) => code.value[i] ?? ''))

function set(i: number, box: HTMLInputElement) {
  const d = box.value.replace(/\D/g, '')
  // Vue leaves the box alone when the code does not change, so a typed letter is wiped here.
  box.value = d.slice(0, 1)
  if (d.length > 1) {
    code.value = d.slice(0, 6)
    boxes.value[Math.min(d.length, 5)]?.focus()
    return
  }
  const next = digits.value.slice()
  next[i] = d
  code.value = next.join('')
  if (d) boxes.value[i + 1]?.focus()
}
function back(i: number, e: KeyboardEvent) {
  if (e.key === 'Backspace' && !digits.value[i]) boxes.value[i - 1]?.focus()
}
defineExpose({ focus: () => boxes.value[0]?.focus() })
</script>

<template>
  <fieldset class="code-boxes" :disabled="disabled">
    <legend class="sr">Authenticator code</legend>
    <input
      v-for="(d, i) in digits"
      :key="i"
      :ref="(el) => (boxes[i] = el as HTMLInputElement)"
      class="input"
      inputmode="numeric"
      :autocomplete="i === 0 ? 'one-time-code' : 'off'"
      :aria-label="`Digit ${i + 1} of 6`"
      :aria-invalid="invalid || undefined"
      :value="d"
      @input="set(i, $event.target as HTMLInputElement)"
      @keydown="back(i, $event)"
      @focus="($event.target as HTMLInputElement).select()"
    />
  </fieldset>
</template>
