export type DocHeading = { id: string; text: string; level: 2 | 3 }
export type DocMeta = {
  path: string
  title: string
  navTitle?: string
  description: string
  section: string
  order: number
  headings: DocHeading[]
  // Drafts are excluded from the manifest, sidebar, sitemap and prerender,
  // so they never appear on the docs site.
  draft?: boolean
}
export { docSections } from './sections'
