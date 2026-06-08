import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, Plus, Trash2, Edit2 } from 'lucide-react'

import {
  useCampaignFormats,
  useCreateCampaignFormat,
  useUpdateCampaignFormat,
  useDeleteCampaignFormat,
} from '../../hooks/useAI'
import type { TeamRecord, CampaignFormat } from '../../types'
import {
  defaultCampaignStructure,
  recordFromStructure,
  structureFromRecord,
  type CampaignStructureFields,
} from './campaignFormatStructure'

interface CampaignFormatViewProps {
  team: TeamRecord
}

const WEEKDAYS = [
  { value: 'null', label: 'Any' },
  { value: '1', label: 'Monday' },
  { value: '2', label: 'Tuesday' },
  { value: '3', label: 'Wednesday' },
  { value: '4', label: 'Thursday' },
  { value: '5', label: 'Friday' },
  { value: '6', label: 'Saturday' },
  { value: '0', label: 'Sunday' },
]

export function CampaignFormatView({ team }: CampaignFormatViewProps) {
  const { data: formats, isLoading } = useCampaignFormats(team.id)
  const createFormat = useCreateCampaignFormat()
  const updateFormat = useUpdateCampaignFormat()
  const deleteFormat = useDeleteCampaignFormat()

  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  
  const [name, setName] = useState('')
  const [weekday, setWeekday] = useState<string>('null')
  const [structureFields, setStructureFields] = useState<CampaignStructureFields>(defaultCampaignStructure())
  const [newSection, setNewSection] = useState('')
  const [advancedMode, setAdvancedMode] = useState(false)
  const [structureJson, setStructureJson] = useState('{}')
  const [hashtags, setHashtags] = useState<string[]>([])
  const [newHashtag, setNewHashtag] = useState('')
  const [isActive, setIsActive] = useState(true)

  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  const resetForm = () => {
    setEditingId(null)
    setName('')
    setWeekday('null')
    setStructureFields(defaultCampaignStructure())
    setStructureJson('{}')
    setAdvancedMode(false)
    setNewSection('')
    setHashtags([])
    setNewHashtag('')
    setIsActive(true)
    setError(null)
  }

  const handleOpenCreate = () => {
    resetForm()
    setIsDialogOpen(true)
  }

  const handleOpenEdit = (format: CampaignFormat) => {
    setEditingId(format.id)
    setName(format.name)
    setWeekday(format.weekday === null ? 'null' : format.weekday.toString())
    const parsed = structureFromRecord(format.structure)
    setStructureFields(parsed.fields)
    setAdvancedMode(parsed.hasAdvancedKeys)
    setStructureJson(JSON.stringify(format.structure, null, 2))
    setHashtags(format.requiredHashtags || [])
    setNewHashtag('')
    setIsActive(format.isActive)
    setError(null)
    setIsDialogOpen(true)
  }

  const handleSave = async () => {
    if (!name.trim()) {
      setError('Name is required')
      return
    }

    let parsedStructure: Record<string, unknown>
    if (advancedMode) {
      try {
        parsedStructure = JSON.parse(structureJson)
      } catch {
        setError('Structure must be valid JSON')
        return
      }
    } else {
      parsedStructure = recordFromStructure(structureFields)
      if (Object.keys(parsedStructure).length === 0) {
        setError('Add at least a topic, tone, section, or instruction')
        return
      }
    }

    setError(null)
    setStatusMessage(null)

    const payload = {
      name: name.trim(),
      weekday: weekday === 'null' ? null : parseInt(weekday, 10),
      structure: parsedStructure,
      required_hashtags: hashtags,
      is_active: isActive,
    }

    try {
      if (editingId) {
        await updateFormat.mutateAsync({
          teamId: team.id,
          formatId: editingId,
          data: payload,
        })
        setStatusMessage('Campaign format updated')
      } else {
        await createFormat.mutateAsync({
          teamId: team.id,
          data: payload,
        })
        setStatusMessage('Campaign format created')
      }
      setIsDialogOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save campaign format')
    }
  }

  const handleDelete = async (formatId: string) => {
    if (!window.confirm('Are you sure you want to delete this campaign format?')) return
    setError(null)
    setStatusMessage(null)
    try {
      await deleteFormat.mutateAsync({ teamId: team.id, formatId })
      setStatusMessage('Campaign format deleted')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete campaign format')
    }
  }

  const handleToggleActive = async (format: CampaignFormat, newActiveState: boolean) => {
    setError(null)
    setStatusMessage(null)
    try {
      await updateFormat.mutateAsync({
        teamId: team.id,
        formatId: format.id,
        data: {
          name: format.name,
          weekday: format.weekday,
          structure: format.structure,
          required_hashtags: format.requiredHashtags,
          is_active: newActiveState,
        },
      })
      setStatusMessage(`Campaign format ${newActiveState ? 'activated' : 'deactivated'}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update campaign format')
    }
  }

  const getWeekdayLabel = (val: number | null | undefined) => {
    if (val == null) return 'Any'
    return WEEKDAYS.find(w => w.value === val.toString())?.label || 'Unknown'
  }

  if (isLoading) {
    return <p className="hint">Loading campaign formats...</p>
  }

  return (
    <div className="stack" data-testid="campaign-format-view">
      <div className="flex-row--between">
        <div>
          <h2 className="section-card__title">Campaign Formats</h2>
          <p className="hint">Define structured templates for AI-generated campaigns.</p>
        </div>
        <button className="btn btn--primary" onClick={handleOpenCreate} data-testid="campaign-create-btn">
          <Plus size={16} />
          <span>Create Format</span>
        </button>
      </div>

      {(error || statusMessage) && !isDialogOpen && (
        <div className="status-banner-panel" style={{ padding: '1rem', marginBottom: '1rem' }}>
          {statusMessage && <span className="status-banner__success" data-testid="campaign-status-success">{statusMessage}</span>}
          {error && <span className="status-banner__error" data-testid="campaign-status-error">{error}</span>}
        </div>
      )}

      <div className="stack stack--sm mt-4" data-testid="campaign-list">
        {formats?.length === 0 ? (
          <div className="empty-state">
            <p className="hint">No campaign formats defined yet.</p>
          </div>
        ) : (
          formats?.map((format) => (
            <div key={format.id} className="glass-panel glass-panel--compact flex-row--between" style={{ alignItems: 'center' }} data-testid={`campaign-format-${format.id}`}>
              <div className="stack stack--xs">
                <div className="flex-row--center gap-2">
                  <h3 style={{ margin: 0, fontSize: '1rem' }}>{format.name}</h3>
                  <span className="badge">{getWeekdayLabel(format.weekday)}</span>
                  <span className={`badge ${format.isActive ? 'badge--success' : 'badge--neutral'}`}>
                    {format.isActive ? 'Active' : 'Inactive'}
                  </span>
                </div>
                <div className="flex-row--center gap-1">
                  {format.requiredHashtags?.map(tag => (
                    <span key={tag} className="badge badge--neutral" style={{ fontSize: '0.7rem' }}>#{tag}</span>
                  ))}
                </div>
              </div>
              
              <div className="flex-row--center gap-3">
                <div className="flex-row--center gap-2" style={{ marginRight: '1rem' }}>
                  <input
                    type="checkbox"
                    checked={format.isActive}
                    onChange={(e) => handleToggleActive(format, e.target.checked)}
                    disabled={updateFormat.isPending}
                  />
                  <span className="hint" style={{ fontSize: '0.8rem' }}>Active</span>
                </div>
                
                <button
                  type="button"
                  className="btn btn--ghost btn--icon-sm"
                  onClick={() => handleOpenEdit(format)}
                >
                  <Edit2 size={16} />
                </button>
                <button
                  type="button"
                  className="btn btn--ghost btn--icon-sm"
                  onClick={() => handleDelete(format.id)}
                  disabled={deleteFormat.isPending}
                >
                  <Trash2 size={16} />
                </button>
              </div>
            </div>
          ))
        )}
      </div>

      <Dialog.Root open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-overlay" />
          <Dialog.Content className="dialog-content" style={{ maxWidth: '600px' }} data-testid="campaign-dialog">
            <div className="drawer-header">
              <Dialog.Title className="drawer-title">
                {editingId ? 'Edit Campaign Format' : 'Create Campaign Format'}
              </Dialog.Title>
              <Dialog.Close asChild>
                <button className="btn btn--ghost btn--icon-sm">
                  <X size={20} />
                </button>
              </Dialog.Close>
            </div>
            <div className="drawer-body stack">
              {error && <div className="status-banner__error mb-4">{error}</div>}
              
              <label className="field">
                <span>Name</span>
                <input
                  data-testid="campaign-dialog-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g., Tech Tuesday, Feature Friday"
                />
              </label>

              <label className="field">
                <span>Target Weekday</span>
                <select data-testid="campaign-dialog-weekday" value={weekday} onChange={(e) => setWeekday(e.target.value)}>
                  {WEEKDAYS.map(w => (
                    <option key={w.value} value={w.value}>{w.label}</option>
                  ))}
                </select>
              </label>

              <div className="field">
                <span>Required Hashtags</span>
                <div className="flex-row--wrap gap-2 mb-2">
                  {hashtags.map((tag, idx) => (
                    <div key={idx} className="badge flex-row--center gap-1">
                      <span>#{tag}</span>
                      <button
                        type="button"
                        className="btn btn--ghost btn--xs"
                        onClick={() => setHashtags(hashtags.filter((_, i) => i !== idx))}
                      >
                        <X size={12} />
                      </button>
                    </div>
                  ))}
                </div>
                <div className="flex-row--center gap-2">
                  <input
                    value={newHashtag}
                    onChange={(e) => setNewHashtag(e.target.value.replace(/^#/, ''))}
                    placeholder="Add hashtag (without #)"
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && newHashtag.trim()) {
                        e.preventDefault()
                        if (!hashtags.includes(newHashtag.trim())) {
                          setHashtags([...hashtags, newHashtag.trim()])
                        }
                        setNewHashtag('')
                      }
                    }}
                  />
                  <button
                    type="button"
                    className="btn btn--secondary"
                    onClick={() => {
                      if (newHashtag.trim() && !hashtags.includes(newHashtag.trim())) {
                        setHashtags([...hashtags, newHashtag.trim()])
                        setNewHashtag('')
                      }
                    }}
                  >
                    Add
                  </button>
                </div>
              </div>

              <div className="field stack stack--sm">
                <div className="flex-row--between" style={{ alignItems: 'center' }}>
                  <span>Content blueprint</span>
                  <label className="flex-row--center gap-2" style={{ fontSize: '0.85rem' }}>
                    <input
                      type="checkbox"
                      checked={advancedMode}
                      onChange={(e) => setAdvancedMode(e.target.checked)}
                    />
                    <span>Advanced JSON</span>
                  </label>
                </div>

                {!advancedMode ? (
                  <>
                    <label className="field">
                      <span>Topic</span>
                      <input
                        value={structureFields.topic}
                        onChange={(e) => setStructureFields((prev) => ({ ...prev, topic: e.target.value }))}
                        placeholder="e.g. product update, community question"
                      />
                    </label>
                    <label className="field">
                      <span>Tone</span>
                      <input
                        value={structureFields.tone}
                        onChange={(e) => setStructureFields((prev) => ({ ...prev, tone: e.target.value }))}
                        placeholder="e.g. informative, playful, concise"
                      />
                    </label>
                    <div className="field">
                      <span>Sections</span>
                      <div className="flex-row--wrap gap-2 mb-2">
                        {structureFields.sections.map((section, idx) => (
                          <div key={idx} className="badge flex-row--center gap-1">
                            <span>{section}</span>
                            <button
                              type="button"
                              className="btn btn--ghost btn--xs"
                              onClick={() =>
                                setStructureFields((prev) => ({
                                  ...prev,
                                  sections: prev.sections.filter((_, i) => i !== idx),
                                }))
                              }
                            >
                              <X size={12} />
                            </button>
                          </div>
                        ))}
                      </div>
                      <div className="flex-row--center gap-2">
                        <input
                          value={newSection}
                          onChange={(e) => setNewSection(e.target.value)}
                          placeholder="Add section, e.g. hook, CTA, takeaway"
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' && newSection.trim()) {
                              e.preventDefault()
                              setStructureFields((prev) => ({
                                ...prev,
                                sections: [...prev.sections, newSection.trim()],
                              }))
                              setNewSection('')
                            }
                          }}
                        />
                        <button
                          type="button"
                          className="btn btn--secondary"
                          onClick={() => {
                            if (!newSection.trim()) return
                            setStructureFields((prev) => ({
                              ...prev,
                              sections: [...prev.sections, newSection.trim()],
                            }))
                            setNewSection('')
                          }}
                        >
                          Add
                        </button>
                      </div>
                    </div>
                    <label className="field">
                      <span>Extra instructions</span>
                      <textarea
                        rows={4}
                        value={structureFields.instructions}
                        onChange={(e) =>
                          setStructureFields((prev) => ({ ...prev, instructions: e.target.value }))
                        }
                        placeholder="Optional guidance for the AI, placeholders like {weekday_name} still work in advanced JSON."
                      />
                    </label>
                  </>
                ) : (
                  <>
                    <p className="hint" style={{ marginTop: '-0.5rem', marginBottom: '0.5rem' }}>
                      Full JSON template. Supported placeholders include {'{weekday_name}'}, {'{day+1}'}, {'{campaign_name}'}.
                    </p>
                    <textarea
                      data-testid="campaign-dialog-structure"
                      rows={8}
                      value={structureJson}
                      onChange={(e) => setStructureJson(e.target.value)}
                      style={{ fontFamily: 'monospace' }}
                    />
                  </>
                )}
              </div>

              <div className="flex-row--center gap-2 mt-2">
                <input
                  type="checkbox"
                  checked={isActive}
                  onChange={(e) => setIsActive(e.target.checked)}
                />
                <span>Active</span>
              </div>

              <div className="flex-row--end gap-2 mt-4">
                <Dialog.Close asChild>
                  <button className="btn btn--ghost">Cancel</button>
                </Dialog.Close>
                <button
                  data-testid="campaign-dialog-save"
                  className="btn btn--primary"
                  onClick={handleSave}
                  disabled={createFormat.isPending || updateFormat.isPending}
                >
                  {createFormat.isPending || updateFormat.isPending ? 'Saving...' : 'Save Format'}
                </button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}
