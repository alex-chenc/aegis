export interface SidebarLayout {
  expandedWidth: number
  wrapMenuLabels: boolean
}

const DEFAULT_EXPANDED_WIDTH = 220
const ENGLISH_EXPANDED_WIDTH = 320

export function getSidebarLayout(locale: string): SidebarLayout {
  const usesLongEnglishLabels = locale.toLowerCase().startsWith('en')

  return {
    expandedWidth: usesLongEnglishLabels ? ENGLISH_EXPANDED_WIDTH : DEFAULT_EXPANDED_WIDTH,
    wrapMenuLabels: usesLongEnglishLabels,
  }
}
