<script setup lang="ts">
import '@fontsource-variable/bricolage-grotesque/opsz.css'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
</script>

<!-- Canvas frame A1, the whole screen: the form on the left, and on a wide screen the promise on the right. -->
<template>
  <div class="grid min-h-dvh grid-cols-2 bg-surface font-body text-[14px]/[1.5] text-ink antialiased [&_*]:box-border max-[720px]:grid-cols-1">
    <div class="grid place-items-center px-10 py-9 max-[720px]:place-items-stretch max-[720px]:px-[max(22px,env(safe-area-inset-left,0px))] max-[720px]:py-7">
      <div class="grid w-full max-w-[400px] content-center gap-3.5">
        <slot />
      </div>
    </div>
    <div aria-hidden="true" class="relative grid content-between gap-6 overflow-hidden bg-ink p-12 text-surface-2 max-[720px]:hidden">
      <span class="absolute -top-32 -right-32 size-[420px] rounded-full border border-surface-2/20" />
      <span class="absolute -top-12 -right-12 size-[260px] rounded-full border border-surface-2/20" />
      <span class="font-mono text-[12px] opacity-70">jorisjonkers.dev</span>
      <p class="m-0 font-display text-[clamp(34px,4vw,56px)]/[1] font-extrabold tracking-[-0.03em]">Sign in <em class="text-accent not-italic">once</em>.</p>
    </div>
  </div>
</template>
