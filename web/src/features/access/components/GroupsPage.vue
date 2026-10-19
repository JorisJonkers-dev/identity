<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import DialogScrim from '../../../shared/DialogScrim.vue'
import Icon from '../../../shared/Icon'
import { useAccess } from '../composables/useAccess'
import { appsOf, groupToShow } from '../model'
import GroupDetail from './GroupDetail.vue'

const route = useRoute()
const { overview, loadError, actionError, createGroup } = useAccess()

const view = computed(() => {
  const o = overview.value
  return o ? { o, group: groupToShow(o, route.params.groupId) } : null
})

const making = ref(false)
const draft = ref('')
function create() {
  createGroup(draft.value, () => undefined)
  making.value = false
}
</script>

<template>
  <AppShell admin>
    <header class="head">
      <h1>Groups</h1>
      <div class="end"><button class="btn primary" type="button" @click="draft = ''; making = true"><Icon name="plus" />New Group</button></div>
    </header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <div v-if="view" class="glass g-split">
      <nav class="g-list" aria-label="Groups">
        <p class="g-list-h">Groups <span>{{ view.o.groups.length }}</span></p>
        <RouterLink v-for="g in view.o.groups" :key="g.id" class="g-item" :to="`/admin/groups/${g.id}`" :aria-current="g.id === view.group.id ? 'page' : undefined">
          <b><Icon v-if="g.admins" name="shield" />{{ g.name }}</b>
          <span v-if="g.requiresTwoFactor" class="gtag tf">Two-factor</span>
          <small>{{ g.members.length }} people · {{ g.admins ? 'Admin console' : `${appsOf(view.o, g.id).length} apps` }}</small>
        </RouterLink>
      </nav>
      <GroupDetail :key="view.group.id" :o="view.o" :group="view.group" />
    </div>

    <DialogScrim v-if="making" @close="making = false">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="g-new" @submit.prevent="create">
        <h2 id="g-new">New Group</h2>
        <div class="field"><label class="label" for="g-name">Name</label><input id="g-name" v-model="draft" class="input md" required maxlength="64" /></div>
        <div class="btn-row end"><button class="btn" type="button" @click="making = false">Cancel</button><button class="btn primary" type="submit">Create Group</button></div>
      </form>
    </DialogScrim>
  </AppShell>
</template>
