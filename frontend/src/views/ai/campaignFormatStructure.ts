export interface CampaignStructureFields {
  topic: string
  tone: string
  sections: string[]
  instructions: string
}

export function defaultCampaignStructure(): CampaignStructureFields {
  return {
    topic: '',
    tone: '',
    sections: [],
    instructions: '',
  }
}

export function structureFromRecord(structure: Record<string, unknown>): {
  fields: CampaignStructureFields
  hasAdvancedKeys: boolean
} {
  const fields = defaultCampaignStructure()
  let hasAdvancedKeys = false

  for (const [key, value] of Object.entries(structure)) {
    if (key === 'topic' && typeof value === 'string') {
      fields.topic = value
      continue
    }
    if (key === 'tone' && typeof value === 'string') {
      fields.tone = value
      continue
    }
    if (key === 'instructions' && typeof value === 'string') {
      fields.instructions = value
      continue
    }
    if (key === 'sections' && Array.isArray(value)) {
      fields.sections = value.map(String).filter(Boolean)
      continue
    }
    hasAdvancedKeys = true
  }

  return { fields, hasAdvancedKeys }
}

export function recordFromStructure(fields: CampaignStructureFields): Record<string, unknown> {
  const structure: Record<string, unknown> = {}
  if (fields.topic.trim()) structure.topic = fields.topic.trim()
  if (fields.tone.trim()) structure.tone = fields.tone.trim()
  if (fields.sections.length > 0) structure.sections = fields.sections.map((item) => item.trim()).filter(Boolean)
  if (fields.instructions.trim()) structure.instructions = fields.instructions.trim()
  return structure
}
