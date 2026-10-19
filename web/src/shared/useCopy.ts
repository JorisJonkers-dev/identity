import { ref } from 'vue'

/** Copies text, and says so for a moment; where the browser refuses, the text stays there to copy by hand. */
export function useCopy() {
  const copied = ref(false)
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      return
    }
    copied.value = true
    setTimeout(() => (copied.value = false), 1400)
  }
  return { copied, copy }
}
