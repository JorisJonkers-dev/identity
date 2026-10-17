/** A link in the sidebar; count, when above zero, shows beside it as a warning. */
export interface SideLink {
  to: string
  label: string
  count?: number
}

/** A group of sidebar links, under a heading when it has one. */
export interface SideSection {
  title?: string
  links: SideLink[]
}
