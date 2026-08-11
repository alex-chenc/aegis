<template>
  <article v-if="report" class="assistant-conclusion" :data-risk="report.riskLevel">
    <header class="conclusion-header">
      <div class="conclusion-heading">
        <span class="conclusion-icon" aria-hidden="true"><el-icon><WarningFilled /></el-icon></span>
        <div>
          <div class="conclusion-eyebrow">{{ report.label }}</div>
          <div class="conclusion-title">{{ report.title }}</div>
        </div>
      </div>
      <span class="risk-badge">{{ report.riskLabel }}</span>
    </header>

    <div class="conclusion-sections">
      <section
        v-for="(section, sectionIndex) in report.sections"
        :key="`${section.title}-${sectionIndex}`"
        class="conclusion-section"
        :class="`section-${section.tone}`"
      >
        <h3>{{ section.title }}</h3>
        <div class="section-content">
          <div
            v-for="(item, itemIndex) in section.items"
            :key="`${sectionIndex}-${itemIndex}`"
            class="section-item"
            :class="{ 'section-item-numbered': item.number !== undefined }"
          >
            <span v-if="item.number !== undefined" class="item-index">{{ item.number }}</span>
            <span class="item-text" v-html="renderInline(item.content)"></span>
          </div>
        </div>
      </section>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'

interface ConclusionItem {
  content: string
  number?: number
}

interface ConclusionSection {
  title: string
  tone: 'summary' | 'risk' | 'action' | 'limit'
  items: ConclusionItem[]
}

interface ConclusionReport {
  label: string
  title: string
  riskLabel: string
  riskLevel: 'critical' | 'high' | 'medium' | 'low' | 'unknown'
  sections: ConclusionSection[]
}

const props = defineProps<{ content: string }>()

const report = computed(() => parseConclusionReport(props.content))

function parseConclusionReport(content: string): ConclusionReport | null {
  const normalized = String(content || '').replace(/\r\n?/g, '\n').trim()
  if (!normalized) return null

  const structured = parseMarkdownSections(normalized)
  const sections = structured.length >= 2 ? structured : parseLegacySections(normalized)
  if (sections.length < 2) return null

  const chinese = /[\u3400-\u9fff]/.test(normalized)
  const riskLevel = inferRiskLevel(normalized)
  const summary = sections.find(section => section.tone === 'summary')?.items[0]?.content || sections[0].items[0]?.content || ''
  return {
    label: chinese ? '安全分析结论' : 'Security analysis',
    title: stripInline(summary).replace(/[。.!！]\s*$/, '') || (chinese ? '风险分析结果' : 'Risk analysis result'),
    riskLabel: localizedRiskLabel(riskLevel, chinese),
    riskLevel,
    sections,
  }
}

function parseMarkdownSections(content: string): ConclusionSection[] {
  const sections: ConclusionSection[] = []
  let current: ConclusionSection | null = null

  for (const rawLine of content.split('\n')) {
    const line = rawLine.trim()
    if (!line) continue
    const heading = line.match(/^#{2,3}\s+(.+)$/)
    if (heading) {
      const title = stripInline(heading[1])
      current = { title, tone: sectionTone(title), items: [] }
      sections.push(current)
      continue
    }
    if (!current) continue
    const numbered = line.match(/^\d+[.)、]\s*(.+)$/)
    const bullet = line.match(/^[-*•]\s*(.+)$/)
    current.items.push({
      content: (numbered?.[1] || bullet?.[1] || line).trim(),
      number: numbered ? Number.parseInt(line, 10) : undefined,
    })
  }

  return sections.filter(section => section.items.length > 0)
}

function parseLegacySections(content: string): ConclusionSection[] {
  if (content.length < 100 || !/(?:安全风险|高风险|risk)/i.test(content)) return []

  const recommendationIndex = findMarker(content, /(?:建议|Recommendations?)[:：]?/i)
  const limitIndex = findMarker(content, /(?:说明|证据边界|Evidence limits?)[:：]?/i)
  const firstEnd = firstSentenceEnd(content)
  const conclusionEnd = firstEnd > 0 ? firstEnd : Math.min(content.length, 180)
  const riskEnd = recommendationIndex >= 0 ? recommendationIndex : (limitIndex >= 0 ? limitIndex : content.length)
  const recommendationEnd = limitIndex >= 0 ? limitIndex : content.length
  const chinese = /[\u3400-\u9fff]/.test(content)

  const sections: ConclusionSection[] = []
  pushLegacySection(sections, chinese ? '结论' : 'Conclusion', 'summary', content.slice(0, conclusionEnd))
  pushLegacySection(sections, chinese ? '具体高风险' : 'Specific high-risk items', 'risk', content.slice(conclusionEnd, riskEnd))
  if (recommendationIndex >= 0) {
    pushLegacySection(sections, chinese ? '处置建议' : 'Recommended actions', 'action', content.slice(recommendationIndex, recommendationEnd).replace(/^(?:建议|Recommendations?)[:：]?\s*/i, ''))
  }
  if (limitIndex >= 0) {
    pushLegacySection(sections, chinese ? '证据边界' : 'Evidence limits', 'limit', content.slice(limitIndex).replace(/^(?:说明|证据边界|Evidence limits?)[:：]?\s*/i, ''))
  }
  return sections
}

function pushLegacySection(sections: ConclusionSection[], title: string, tone: ConclusionSection['tone'], content: string) {
  const value = content.trim()
  if (!value) return
  sections.push({ title, tone, items: [{ content: value }] })
}

function sectionTone(title: string): ConclusionSection['tone'] {
  if (/(?:高风险|风险项|specific.*risk|findings?)/i.test(title)) return 'risk'
  if (/(?:建议|处置|recommend|action)/i.test(title)) return 'action'
  if (/(?:边界|限制|缺口|limit|gap|coverage)/i.test(title)) return 'limit'
  return 'summary'
}

function inferRiskLevel(content: string): ConclusionReport['riskLevel'] {
  if (/(?:严重|极高|critical)/i.test(content)) return 'critical'
  if (/(?:高风险|风险等级\s*[:：]?\s*高|risk\s*(?:level)?\s*[:：]?\s*high)/i.test(content)) return 'high'
  if (/(?:中风险|风险等级\s*[:：]?\s*中|medium\s*risk)/i.test(content)) return 'medium'
  if (/(?:低风险|风险等级\s*[:：]?\s*低|low\s*risk)/i.test(content)) return 'low'
  return 'unknown'
}

function localizedRiskLabel(level: ConclusionReport['riskLevel'], chinese: boolean): string {
  const labels = chinese
    ? { critical: '严重风险', high: '高风险', medium: '中风险', low: '低风险', unknown: '待确认' }
    : { critical: 'Critical', high: 'High risk', medium: 'Medium risk', low: 'Low risk', unknown: 'Unverified' }
  return labels[level]
}

function findMarker(content: string, pattern: RegExp): number {
  const match = pattern.exec(content)
  return match?.index ?? -1
}

function firstSentenceEnd(content: string): number {
  const punctuation = /[。.!！]/g
  let match: RegExpExecArray | null

  while ((match = punctuation.exec(content))) {
    const isDecimalOrIpDot = match[0] === '.' && /\d/.test(content[match.index - 1] || '') && /\d/.test(content[match.index + 1] || '')
    if (!isDecimalOrIpDot) return match.index + match[0].length
  }

  return -1
}

function stripInline(content: string): string {
  return content.replace(/\*\*(.*?)\*\*/g, '$1').replace(/`(.*?)`/g, '$1').trim()
}

function renderInline(content: string): string {
  return escapeHtml(content)
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
}

function escapeHtml(content: string): string {
  return content
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}
</script>

<style scoped>
.assistant-conclusion {
  overflow: hidden;
  border: 1px solid #fecaca;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 12px 32px rgba(127, 29, 29, 0.08);
}

.conclusion-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px;
  border-bottom: 1px solid #fee2e2;
  background: linear-gradient(135deg, #fff7ed 0%, #fef2f2 100%);
}

.conclusion-heading {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.conclusion-icon {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 10px;
  background: #fee2e2;
  color: #dc2626;
  font-size: 20px;
}

.conclusion-eyebrow {
  margin-bottom: 3px;
  color: #991b1b;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.conclusion-title {
  overflow: hidden;
  color: #1f2937;
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risk-badge {
  flex: 0 0 auto;
  padding: 5px 10px;
  border: 1px solid #fca5a5;
  border-radius: 999px;
  background: #fff;
  color: #b91c1c;
  font-size: 12px;
  font-weight: 700;
}

.assistant-conclusion[data-risk='medium'] {
  border-color: #fde68a;
}

.assistant-conclusion[data-risk='medium'] .conclusion-header {
  border-bottom-color: #fef3c7;
  background: linear-gradient(135deg, #fffbeb 0%, #fff7ed 100%);
}

.assistant-conclusion[data-risk='low'] {
  border-color: #bbf7d0;
}

.conclusion-sections {
  display: grid;
  gap: 12px;
  padding: 14px;
}

.conclusion-section {
  padding: 13px 14px;
  border: 1px solid #e5e7eb;
  border-left-width: 4px;
  border-radius: 10px;
  background: #f8fafc;
}

.conclusion-section h3 {
  margin: 0 0 9px;
  color: #111827;
  font-size: 14px;
  font-weight: 750;
}

.section-summary { border-left-color: #64748b; }
.section-risk { border-left-color: #ef4444; background: #fffafa; }
.section-action { border-left-color: #3b82f6; background: #f8fbff; }
.section-limit { border-left-color: #f59e0b; background: #fffdf5; }

.section-content {
  display: grid;
  gap: 8px;
}

.section-item {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  color: #334155;
  font-size: 13px;
  line-height: 1.7;
}

.section-item:not(.section-item-numbered)::before {
  width: 5px;
  height: 5px;
  margin-top: 8px;
  flex: 0 0 5px;
  border-radius: 999px;
  background: #94a3b8;
  content: '';
}

.item-index {
  display: grid;
  width: 21px;
  height: 21px;
  flex: 0 0 21px;
  place-items: center;
  border-radius: 7px;
  background: #fee2e2;
  color: #b91c1c;
  font-size: 11px;
  font-weight: 750;
}

.item-text {
  min-width: 0;
  word-break: break-word;
}

.item-text :deep(code) {
  padding: 2px 5px;
  border-radius: 4px;
  background: #e2e8f0;
  color: #334155;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

@media (max-width: 720px) {
  .conclusion-header {
    align-items: flex-start;
    padding: 14px;
  }

  .conclusion-title {
    white-space: normal;
  }

  .conclusion-sections {
    padding: 10px;
  }
}
</style>
