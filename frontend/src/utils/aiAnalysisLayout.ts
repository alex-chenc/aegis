const DEFAULT_FILTER_LABEL_WIDTH = 80
const ENGLISH_FILTER_LABEL_WIDTH = 190

export function getAIAnalysisFilterLabelWidth(locale: string): number {
  return locale.toLowerCase().startsWith('en')
    ? ENGLISH_FILTER_LABEL_WIDTH
    : DEFAULT_FILTER_LABEL_WIDTH
}
