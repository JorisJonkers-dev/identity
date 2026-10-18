<script setup lang="ts">
import '@fontsource-variable/bricolage-grotesque/opsz.css'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'

// glyph is the card's sign: a lock for proving it is you, a mark for something that stops you.
defineProps<{ glyph: 'lock' | 'stop' | 'mail' }>()
</script>

<!-- Canvas frames A2 and A3: one card in the middle of the page. -->
<template>
  <div class="grid min-h-dvh place-items-center bg-surface-2 px-5 py-8 font-body text-[14px]/[1.5] text-ink antialiased [&_*]:box-border">
    <div class="grid w-full max-w-[420px] gap-3.5 rounded-2xl border border-line bg-surface p-7 shadow-[0_1px_2px_rgb(13_22_38/6%),0_12px_32px_-12px_rgb(13_22_38/18%)]">
      <div :class="['grid size-11 place-items-center rounded-xl', glyph === 'stop' ? 'bg-warn-soft text-warn' : 'bg-accent-soft text-accent']" aria-hidden="true">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <template v-if="glyph === 'lock'"><rect x="5" y="10" width="14" height="10" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" /></template>
          <template v-else-if="glyph === 'stop'"><circle cx="12" cy="12" r="9" /><path d="M12 7v6M12 16.5v.5" /></template>
          <template v-else><rect x="3" y="5" width="18" height="14" rx="2" /><path d="m3 7 9 6 9-6" /></template>
        </svg>
      </div>
      <slot />
    </div>
  </div>
</template>
