/** How the Launcher greets someone at a time of day: morning until noon, afternoon until six. */
export function greeting(name: string | undefined, at: Date): string {
  const hour = at.getHours()
  const part = hour < 12 ? 'morning' : hour < 18 ? 'afternoon' : 'evening'
  const first = name?.trim().split(/\s+/)[0]
  return first ? `Good ${part}, ${first}` : `Good ${part}`
}
