<script setup lang="ts">
import Icon from './Icon'

/** A role: filled with a check when held, dashed when not, locked when it comes from elsewhere. */
defineProps<{ role: string; held: boolean; locked?: boolean; title?: string }>()
defineEmits<{ toggle: [] }>()
</script>

<template>
  <button
    type="button"
    :class="['chip', { via: locked }]"
    :aria-pressed="held"
    :disabled="locked"
    :title="title"
    @click="$emit('toggle')"
  >
    <Icon :name="locked ? 'lock' : 'check'" />{{ role }}
  </button>
</template>
