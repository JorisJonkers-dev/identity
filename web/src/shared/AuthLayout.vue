<script setup lang="ts">
import '@fontsource-variable/bricolage-grotesque/opsz.css'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
</script>

<!-- Canvas frame A1: the form on the left, and on a wide screen the promise on the right. -->
<template>
  <div class="grid min-h-dvh place-items-center bg-surface-2 p-6 font-body text-[14px]/[1.5] text-ink antialiased [&_*]:box-border max-[720px]:place-items-stretch max-[720px]:p-0">
    <div
      class="grid w-full max-w-[920px] grid-cols-2 overflow-hidden rounded-2xl border border-line bg-surface shadow-[0_1px_2px_rgb(13_22_38/6%),0_12px_32px_-12px_rgb(13_22_38/18%)] max-[720px]:max-w-none max-[720px]:grid-cols-1 max-[720px]:rounded-none max-[720px]:border-0 max-[720px]:shadow-none"
    >
      <div class="grid min-h-[470px] content-center gap-3.5 px-10 py-9 max-[720px]:min-h-dvh max-[720px]:px-[max(22px,env(safe-area-inset-left,0px))] max-[720px]:py-7">
        <slot />
      </div>
      <div aria-hidden="true" class="relative grid content-between gap-6 overflow-hidden bg-ink p-8 text-surface-2 max-[720px]:hidden">
        <span class="absolute -top-20 -right-20 size-[260px] rounded-full border border-surface-2/20" />
        <span class="absolute -top-[30px] -right-[30px] size-40 rounded-full border border-surface-2/20" />
        <span class="font-mono text-[12px] opacity-70">jorisjonkers.dev</span>
        <p class="m-0 font-display text-[34px]/[1] font-extrabold tracking-[-0.03em]">Sign in once.<br /><em class="text-accent not-italic">Every app</em> knows you.</p>
      </div>
    </div>
  </div>
</template>
