/**
 * Suggested Tag colours. A Tag's colour is the user's free choice, so
 * these are data rather than design tokens: they seed a newly typed Tag
 * and sit next to the custom colour picker in the Tags view. Picked to stay
 * clear of purple (the accent) and of the `state-*` tunnel colours.
 */
export const TAG_SWATCHES = [
  '#38bdf8', // sky-400
  '#818cf8', // indigo-400
  '#f472b6', // pink-400
  '#fb923c', // orange-400
  '#2dd4bf', // teal-400
  '#94a3b8', // slate-400
] as const

/** Mirrors store.MaxTagLabel. */
export const MAX_TAG_LABEL = 24

/** The swatch a new Tag gets, rotating so neighbours rarely share one. */
export function nextSwatch(n: number): string {
  return TAG_SWATCHES[n % TAG_SWATCHES.length]
}

/**
 * A Tag as the connection form holds it: an existing Tag has an id, one
 * typed in the form does not until the connection is saved.
 */
export type TagRef = {
  id?: number
  label: string
  color: string
}

export function sameLabel(a: string, b: string): boolean {
  return a.trim().toLocaleLowerCase() === b.trim().toLocaleLowerCase()
}
