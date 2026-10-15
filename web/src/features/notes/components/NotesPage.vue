<script setup lang="ts">
import { computed, ref } from 'vue'
import { useNotes } from '../composables/useNotes'
import { checkNoteText, describeNoteTextCheck, MAX_NOTE_LENGTH } from '../domain/noteText'

const draft = ref('')
const { notes, isLoading, loadError, isCreating, createError, create } = useNotes()
const check = computed(() => checkNoteText(draft.value))
const hint = computed(() => (draft.value === '' ? null : describeNoteTextCheck(check.value)))

function submit(): void {
  if (check.value.kind !== 'ok') return
  create(check.value.text, () => {
    draft.value = ''
  })
}

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <section class="notes">
    <h1>Notes</h1>

    <form class="compose" @submit.prevent="submit">
      <label for="note-text">New note</label>
      <div class="row">
        <input
          id="note-text"
          v-model="draft"
          type="text"
          autocomplete="off"
          aria-describedby="note-hint"
        />
        <button type="submit" :disabled="check.kind !== 'ok' || isCreating">Add note</button>
      </div>
      <p id="note-hint" class="hint">{{ hint ?? `Up to ${String(MAX_NOTE_LENGTH)} characters.` }}</p>
      <p v-if="createError" role="alert" class="error">{{ createError }}</p>
    </form>

    <p v-if="isLoading">Loading notes…</p>
    <p v-else-if="loadError" role="alert" class="error">{{ loadError }}</p>
    <p v-else-if="notes.length === 0">No notes yet.</p>
    <ul v-else aria-label="Notes" class="list">
      <li v-for="note in notes" :key="note.id">
        <span>{{ note.text }}</span>
        <time :datetime="note.createdAt">{{ dateFormat.format(new Date(note.createdAt)) }}</time>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.notes {
  display: grid;
  gap: 1rem;
}
.compose {
  display: grid;
  gap: 0.25rem;
}
.row {
  display: flex;
  gap: 0.5rem;
}
input {
  flex: 1;
  min-width: 0;
  padding: 0.5rem;
  font: inherit;
}
button {
  padding: 0.5rem 1rem;
  font: inherit;
}
.hint {
  margin: 0;
  color: #4a4a4a;
  font-size: 0.875rem;
}
.error {
  color: #a00000;
}
.list {
  display: grid;
  gap: 0.5rem;
  padding: 0;
  list-style: none;
}
.list li {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid #d0d0d0;
}
time {
  color: #4a4a4a;
  white-space: nowrap;
}
</style>
