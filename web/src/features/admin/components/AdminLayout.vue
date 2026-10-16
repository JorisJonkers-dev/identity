<script setup lang="ts">
import '@fontsource-variable/bricolage-grotesque/opsz.css'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import { useQuery } from '@tanstack/vue-query'
import { computed, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { getDashboardOptions, getProfileOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { initials } from '../format'

// An Admin whose session proved no Second Factor is sent to add one first (docs/adr/0005).
const router = useRouter()
const profile = useQuery({ ...getProfileOptions(), retry: false })
watch(
  () => profile.data.value,
  (p) => {
    if (p?.admin && !p.secondFactor) void router.replace({ path: '/security', query: { for: 'admin' } })
  },
  { immediate: true },
)
const dashboard = useQuery({ ...getDashboardOptions(), retry: false })
const waiting = computed(() => dashboard.data.value?.unassigned.length ?? 0)
</script>

<template>
  <div class="admin">
    <nav class="side" aria-label="Admin">
      <div v-if="profile.data.value" class="who">
        <span class="av" aria-hidden="true">{{ initials(profile.data.value.name) }}</span>
        <div>
          <b>{{ profile.data.value.name }}</b>
          <small>Admin · 12h session</small>
        </div>
      </div>
      <span class="sep">Admin</span>
      <RouterLink to="/admin" exact-active-class="on">Dashboard</RouterLink>
      <RouterLink to="/access#waiting-title">
        Unassigned <span v-if="waiting > 0" class="count">{{ waiting }}</span>
      </RouterLink>
      <RouterLink to="/access">Groups and access</RouterLink>
      <RouterLink to="/admin/audit" exact-active-class="on">Audit</RouterLink>
      <span class="sep">You</span>
      <RouterLink to="/">Your apps</RouterLink>
    </nav>
    <div class="main">
      <slot />
    </div>
  </div>
</template>

<style>
/* The admin pages, as the design canvas drew them (C1 Dashboard, C4 Audit). */
.admin {
  --surface: #ffffff;
  --surface-2: #f4f6fa;
  --line: #dde3ec;
  --ink: #0d1626;
  --ink-2: #485468;
  /* Darker than the canvas's #7a879b, which falls short of WCAG AA for small text. */
  --ink-3: #5f6b80;
  --accent: #2545f0;
  --accent-ink: #ffffff;
  --accent-soft: #e5e9ff;
  --ok: #10805a;
  --ok-soft: #e1f3ea;
  --warn: #9a5a00;
  --warn-soft: #fbefd8;
  --bad: #bf2630;
  --bad-soft: #fbe4e5;
  --display: 'Bricolage Grotesque Variable', 'Avenir Next', 'Segoe UI', system-ui, sans-serif;
  --body: 'Geist Variable', 'Segoe UI', system-ui, -apple-system, sans-serif;
  --mono: 'Geist Mono Variable', ui-monospace, 'SF Mono', Menlo, Consolas, monospace;

  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  min-height: 100vh;
  background: var(--surface-2);
  color: var(--ink);
  font: 14px/1.5 var(--body);
  -webkit-font-smoothing: antialiased;
}
@media (prefers-color-scheme: dark) {
  .admin {
    --surface: #111823;
    --surface-2: #172030;
    --line: #253043;
    --ink: #e7ecf4;
    --ink-2: #a8b3c4;
    --ink-3: #8a97ad;
    --accent: #7f95ff;
    --accent-ink: #0a0e15;
    --accent-soft: #1b244c;
    --ok: #4ccb9a;
    --ok-soft: #102c20;
    --warn: #e8a94b;
    --warn-soft: #33250e;
    --bad: #ff7d84;
    --bad-soft: #3a1519;
    color-scheme: dark;
  }
}
.admin *,
.admin *::before,
.admin *::after {
  box-sizing: border-box;
}
.admin :focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
  border-radius: 6px;
}
.admin .side {
  border-right: 1px solid var(--line);
  padding: 16px 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: var(--surface);
}
.admin .side .who {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 6px 8px 16px;
}
.admin .who small {
  display: block;
  color: var(--ink-3);
  font-size: 11.5px;
}
.admin .side a {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 7px 10px;
  border-radius: 8px;
  color: var(--ink-2);
  text-decoration: none;
  font-weight: 500;
  font-size: 13px;
}
.admin .side a.on {
  background: var(--accent-soft);
  color: var(--accent);
}
.admin .side .sep {
  font: 500 10.5px var(--mono);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-3);
  padding: 14px 10px 4px;
}
.admin .count {
  font: 500 11px var(--mono);
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--warn-soft);
  color: var(--warn);
}
.admin .main {
  padding: 22px 24px;
  min-width: 0;
  display: grid;
  gap: 18px;
  align-content: start;
}
.admin h1 {
  font: 700 22px/1.15 var(--display);
  letter-spacing: -0.02em;
  margin: 0;
  text-wrap: balance;
}
.admin .sub {
  color: var(--ink-3);
  margin: 2px 0 0;
  font-size: 13px;
}
.admin .head {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  justify-content: space-between;
  align-items: end;
}
.admin .av {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font: 600 12px var(--body);
  background: var(--accent-soft);
  color: var(--accent);
  flex: none;
}
.admin .av.s {
  width: 24px;
  height: 24px;
  font-size: 10.5px;
}
.admin .btn {
  font: 600 13px var(--body);
  border-radius: 9px;
  padding: 8px 14px;
  border: 1px solid var(--line);
  background: var(--surface);
  color: var(--ink);
  cursor: pointer;
  white-space: nowrap;
  text-decoration: none;
}
.admin .panel {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 16px;
  min-width: 0;
}
.admin .panel h2 {
  margin: 0 0 10px;
  font: 600 13px var(--body);
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.admin .panel h2 span {
  color: var(--ink-3);
  font-weight: 400;
}
.admin .pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font: 500 11.5px var(--mono);
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--surface-2);
  color: var(--ink-2);
  border: 1px solid var(--line);
  white-space: nowrap;
}
.admin .pill.a {
  background: var(--accent-soft);
  color: var(--accent);
  border-color: transparent;
}
.admin .list {
  display: grid;
  margin: 0;
  padding: 0;
  list-style: none;
}
.admin .list > li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--line);
  min-width: 0;
}
.admin .list > li:first-child {
  border-top: 0;
  padding-top: 0;
}
.admin .list .grow {
  flex: 1;
  min-width: 0;
}
.admin .list .grow small {
  display: block;
  color: var(--ink-3);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.admin .muted {
  color: var(--ink-3);
  margin: 0;
}
.admin .error {
  color: var(--bad);
  margin: 0;
}
.admin .ext {
  color: var(--accent);
  text-decoration: none;
  font-size: 12.5px;
  font-weight: 500;
}
@media (max-width: 720px) {
  .admin {
    grid-template-columns: minmax(0, 1fr);
  }
  /* The sidebar becomes a row of links the width of the phone, scrolling sideways on its own. */
  .admin .side {
    flex-direction: row;
    align-items: center;
    overflow-x: auto;
    border-right: 0;
    border-bottom: 1px solid var(--line);
    padding: 10px max(16px, env(safe-area-inset-left, 0px));
  }
  .admin .side .who,
  .admin .side .sep {
    display: none;
  }
  .admin .side a {
    flex: none;
    gap: 6px;
  }
  .admin .main {
    padding: 18px max(16px, env(safe-area-inset-right, 0px)) 18px max(16px, env(safe-area-inset-left, 0px));
  }
}
</style>
