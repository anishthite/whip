export const docRedirects: Readonly<Record<string, string>> = {
  '/': '/docs/quickstart',
  '/docs': '/docs/quickstart',
  '/docs/introduction': '/docs/quickstart',
  '/docs/getting-started': '/docs/quickstart',
  '/docs/installation': '/docs/download',
  // The TUI and permissions pages are currently drafts; redirect their legacy
  // URLs to the closest published page until they are published again.
  '/docs/using-whipcode/cli': '/docs/quickstart',
  '/docs/tools-and-permissions': '/docs/quickstart',
}

export function docRedirect(pathname: string) {
  const clean = pathname.replace(/(?:\/index\.html|\.html|\/+)$/, '') || '/'
  return docRedirects[clean]
}
