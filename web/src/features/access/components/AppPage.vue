<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { rotateClientSecretMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { AccessOverview } from '../../../infrastructure/api/types.gen'
import { useAction } from '../../../infrastructure/useAction'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import { useAccess } from '../composables/useAccess'
import { useStepUp } from '../composables/useStepUp'
import { minutesOf, stepUpChoices, stepUpSummary } from '../stepUp'
import { holders } from '../model'

const route = useRoute()
const { overview, loadError, actionError, setRole } = useAccess()
const clientId = computed(() => String(route.params.clientId))
const app = computed(() => overview.value?.clients.find((c) => c.id === clientId.value))
const rotate = useAction(rotateClientSecretMutation())
const asking = ref(false)
const stepUp = useStepUp(clientId)
const stepUpChoice = computed(() => String(stepUp.minutes.value ?? 0))
function pickStepUp(choice: string) {
  stepUp.set(minutesOf(choice))
}
const addable = (o: AccessOverview, role: string) => {
  return o.groups.filter((g) => !g.admins && !holders(o, clientId.value, role).some((h) => h.id === g.id)).map((g) => ({ value: g.id, label: g.name }))
}
function rotateNow() {
  asking.value = false
  rotate.run({ path: { clientId: clientId.value } })
}
</script>

<template>
  <AppShell admin narrow>
    <BackLink to="/admin/apps" label="Apps" />
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <template v-if="overview && app">
      <header class="app-head glass"><AppIcon :label="app.label" size="xl" /><div><h1>{{ app.label }}</h1><span class="mono soft">{{ app.id }}</span></div></header>
      <section class="glass group">
        <div class="group-h"><h2>Roles</h2></div>
        <div v-for="r in app.roles" :key="r" class="role-block">
          <div class="rb-h"><span class="role big">{{ r }}</span><span class="soft mono">{{ holders(overview, app.id, r).length }} Groups</span></div>
          <div class="rb-line">
            <span class="rb-k"><Icon name="users" />Groups</span>
            <div class="gchips gchips-inline">
              <span v-for="g in holders(overview, app.id, r)" :key="g.id" class="gchip">{{ g.name }}<button type="button" :aria-label="`Take ${r} from ${g.name}`" @click="setRole(g.id, app.id, r, false)"><Icon name="x" /></button></span>
              <span v-if="holders(overview, app.id, r).length === 0" class="soft">None</span>
            </div>
            <GlassSelect :options="addable(overview, r)" :label="`Give ${r} to a Group`" placeholder="Add a Group" small @pick="(g) => setRole(g, app!.id, r, true)" />
          </div>
        </div>
      </section>
      <section class="glass group" style="margin-top: 16px">
        <div class="group-h"><h2>Sign-in</h2></div>
        <div class="row">
          <span class="row-ic"><Icon name="key" /></span>
          <span class="grow"><b>App secret</b><small>{{ rotate.isDone.value ? 'Secret replaced' : 'Kept in Vault' }}</small></span>
          <button v-if="!asking" class="btn sm" type="button" :disabled="rotate.isPending.value" @click="asking = true">Replace secret</button>
        </div>
        <div v-if="asking" class="confirm">
          <div class="ask">Replace the secret for {{ app.label }}?<small>The old one works until {{ app.label }} uses the new one.</small></div>
          <button class="btn sm" type="button" @click="asking = false">Cancel</button>
          <button class="btn sm primary" type="button" @click="rotateNow">Replace secret</button>
        </div>
        <p v-if="rotate.error.value" role="alert" class="alert"><Icon name="alert" />{{ rotate.error.value }}</p>
        <div class="row step-up">
          <span class="row-ic"><Icon name="shield" /></span>
          <span class="grow"><b>Ask for two-factor again</b><small>{{ stepUpSummary(stepUp.minutes.value, app.label) }}</small></span>
          <GlassSelect :model-value="stepUpChoice" :options="stepUpChoices" :label="`Ask for two-factor again in ${app.label}`" small @pick="pickStepUp" />
        </div>
        <p v-if="stepUp.error.value" role="alert" class="alert"><Icon name="alert" />{{ stepUp.error.value }}</p>
      </section>
    </template>
  </AppShell>
</template>
