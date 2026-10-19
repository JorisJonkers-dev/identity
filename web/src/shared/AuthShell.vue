<script setup lang="ts">
import { RouterLink } from 'vue-router'
import Icon from './Icon'
import type { IconName } from './icons'

/** The signed-out pages: the form on the left, the estate drawn as an orbit on a glass pane. */
defineProps<{ brandless?: boolean }>()
const nodes: { at: [number, number]; icon: IconName }[] = [
  { at: [94.5, 156], icon: 'monitor' },
  { at: [240, 72], icon: 'apps' },
  { at: [385.5, 156], icon: 'users' },
  { at: [385.5, 324], icon: 'key' },
  { at: [240, 408], icon: 'mail' },
  { at: [94.5, 324], icon: 'lock' },
]
</script>

<template>
  <div class="auth">
    <main class="auth-side">
      <RouterLink class="brand" to="/" :class="{ hidden: brandless }"><span class="mark"><Icon name="mark" /></span>jorisjonkers.dev</RouterLink>
      <slot />
    </main>
    <div class="auth-art" aria-hidden="true">
      <svg class="orbit" viewBox="0 0 480 480">
        <circle class="ring" cx="240" cy="240" r="168" />
        <circle class="ring faint" cx="240" cy="240" r="104" />
        <line v-for="n in nodes" :key="`l${n.icon}`" class="spoke" x1="240" y1="240" :x2="n.at[0]" :y2="n.at[1]" />
        <g v-for="n in nodes" :key="n.icon" class="node">
          <rect :x="n.at[0] - 26" :y="n.at[1] - 26" width="52" height="52" rx="15" />
          <svg :x="n.at[0] - 12" :y="n.at[1] - 12" width="24" height="24"><Icon :name="n.icon" /></svg>
        </g>
        <circle class="core-ring" cx="240" cy="240" r="56" />
        <circle class="core-ring" cx="240" cy="240" r="72" stroke-opacity=".07" />
        <rect x="204" y="204" width="72" height="72" rx="21" fill="var(--accent-solid)" />
        <rect x="204.5" y="204.5" width="71" height="71" rx="20.5" fill="none" stroke="rgb(255 255 255 / 0.22)" />
        <svg x="220" y="219" width="40" height="40" style="color: #fff"><Icon name="mark" /></svg>
      </svg>
      <p class="si-line">Sign in once.</p>
    </div>
  </div>
</template>
