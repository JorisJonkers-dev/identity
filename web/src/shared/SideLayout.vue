<script setup lang="ts">
import '@fontsource-variable/bricolage-grotesque/opsz.css'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import { RouterLink } from 'vue-router'
import { initials } from './initials'
import type { SideSection } from './side'

// label names the sidebar for a screen reader; name is who is signed in, once they are known.
defineProps<{ label: string; name?: string; detail?: string; sections: SideSection[] }>()

const link =
  'flex items-center justify-between rounded-[10px] px-3 py-2 text-[14.5px] font-medium text-ink-2 no-underline transition-colors hover:bg-surface-2 hover:text-ink max-[720px]:flex-none max-[720px]:gap-1.5'
const open = 'bg-accent-soft text-accent!'
</script>

<!-- The canvas's page frame: a sidebar of who is signed in and where they can go, beside the page. -->
<template>
  <div
    class="grid min-h-dvh grid-cols-[260px_minmax(0,1fr)] bg-surface-2 font-body text-[15px]/[1.55] text-ink antialiased [color-scheme:light] max-[720px]:grid-cols-1 dark:[color-scheme:dark] [&_*]:box-border [&_:focus-visible]:rounded-md [&_:focus-visible]:outline-2 [&_:focus-visible]:outline-offset-2 [&_:focus-visible]:outline-accent"
  >
    <nav
      class="flex flex-col gap-1 border-r border-line bg-surface px-4 py-6 max-[720px]:flex-row max-[720px]:items-center max-[720px]:overflow-x-auto max-[720px]:border-r-0 max-[720px]:border-b max-[720px]:px-[max(16px,env(safe-area-inset-left,0px))] max-[720px]:py-2.5"
      :aria-label="label"
    >
      <div v-if="name" data-who class="flex items-center gap-3 px-2 pt-1 pb-6 max-[720px]:hidden">
        <span class="grid size-10 flex-none place-items-center rounded-full bg-accent-soft text-[14px] font-semibold text-accent" aria-hidden="true">
          {{ initials(name) }}
        </span>
        <div class="min-w-0">
          <b class="block truncate">{{ name }}</b>
          <small class="block truncate text-[12.5px] text-ink-3" :title="detail">{{ detail }}</small>
        </div>
      </div>
      <template v-for="(section, i) in sections" :key="i">
        <span v-if="section.title" class="eyebrow px-3 pt-5 pb-1.5 max-[720px]:hidden">{{ section.title }}</span>
        <RouterLink v-for="l in section.links" :key="l.to" :to="l.to" :class="link" :exact-active-class="open">
          {{ l.label }}
          <span v-if="l.count" data-count class="rounded-full bg-warn-soft px-[7px] py-px font-mono text-[11px] font-medium text-warn">{{ l.count }}</span>
        </RouterLink>
      </template>
    </nav>
    <div
      class="mx-auto grid w-full max-w-[1280px] min-w-0 content-start gap-7 px-10 py-9 max-[720px]:py-[18px] max-[720px]:pr-[max(16px,env(safe-area-inset-right,0px))] max-[720px]:pl-[max(16px,env(safe-area-inset-left,0px))]"
    >
      <slot />
    </div>
  </div>
</template>
