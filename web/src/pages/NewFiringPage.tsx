import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { Alert, ConeSelect, Field, inputClass, ShareInput } from '../components/firing'
import { createFiring, getOptions } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { earliestDay } from '../lib/time'
import type { Options, Profile } from '../lib/types'
import { useLoad } from '../lib/useLoad'

export default function NewFiringPage() {
  const { data: options, error } = useLoad(getOptions, [])
  if (error) return <Alert kind="error">{error}</Alert>
  if (!options) return <PageSpinner />
  return <ProposeForm options={options} />
}

function firstProfile(profiles: Profile[], kilnId: number): Profile | undefined {
  return profiles.find((p) => p.kiln_id === kilnId)
}

function ProposeForm({ options }: { options: Options }) {
  const { me } = useSession()
  const navigate = useNavigate()
  const canBypass = !!me && (me.roles.kiln_master || me.roles.admin)
  const canSaveProfile = canBypass

  const activeKilns = options.kilns.filter((k) => k.active)
  const [kilnId, setKilnId] = useState(activeKilns[0]?.id ?? 0)
  const initialProfile = firstProfile(options.profiles, kilnId)
  const [profileId, setProfileId] = useState(initialProfile ? String(initialProfile.id) : '__other')
  const [customName, setCustomName] = useState(initialProfile?.name ?? '')
  const [customType, setCustomType] = useState(initialProfile?.firing_type ?? options.firing_types[0] ?? 'glaze')
  const [customCone, setCustomCone] = useState(initialProfile?.cone ?? options.cones[0] ?? '6')
  const [customFiringHours, setCustomFiringHours] = useState(initialProfile ? String(initialProfile.firing_minutes / 60) : '10')
  const [customCooldownHours, setCustomCooldownHours] = useState(initialProfile ? String(initialProfile.cooldown_minutes / 60) : '14')
  const [saveProfile, setSaveProfile] = useState(false)
  // Firings are booked by day. Members start from the earliest day their notice allows.
  const earliest = earliestDay(options.timezone, options.min_notice_hours)
  const [day, setDay] = useState(earliest)
  const [description, setDescription] = useState('')
  const [share, setShare] = useState(25)
  const [clayBody, setClayBody] = useState('')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const kilnProfiles = options.profiles.filter((p) => p.kiln_id === kilnId)
  const selected = kilnProfiles.find((p) => String(p.id) === profileId)
  const other = profileId === '__other'

  // Changing kiln resets the profile choice to that kiln's first preset.
  useEffect(() => {
    const first = firstProfile(options.profiles, kilnId)
    setProfileId(first ? String(first.id) : '__other')
    if (first) {
      setCustomName(first.name)
      setCustomType(first.firing_type)
      setCustomCone(first.cone)
      setCustomFiringHours(String(first.firing_minutes / 60))
      setCustomCooldownHours(String(first.cooldown_minutes / 60))
    }
    // Only on kiln change; the profile values are handled with the selection.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kilnId])

  // Days this kiln is blacked out (maintenance and other needs). YYYY-MM-DD strings compare correctly.
  const blackouts = options.blackouts.filter((b) => b.kiln_id === kilnId)
  const blocked = blackouts.some((b) => day >= b.start_date && day <= b.end_date)

  const firingCone = selected?.cone ?? customCone
  const firingMinutes = selected?.firing_minutes ?? Math.round(Number(customFiringHours) * 60)
  const cooldownMinutes = selected?.cooldown_minutes ?? Math.round(Number(customCooldownHours) * 60)

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!day) return setErr('Pick a day.')
    setBusy(true)
    setErr(null)
    try {
      const d = await createFiring({
        kiln_id: kilnId,
        profile_id: selected ? selected.id : null,
        custom: other
          ? {
              name: customName,
              firing_type: customType,
              cone: customCone,
              firing_minutes: firingMinutes,
              cooldown_minutes: cooldownMinutes,
            }
          : null,
        notes,
        scheduled_date: day,
        description,
        share_pct: share,
        proposer_cone: firingCone,
        clay_body: clayBody || null,
        save_profile: other && canSaveProfile && saveProfile,
      })
      navigate(`/firings/${d.firing.id}`, { replace: true })
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="space-y-5">
      <h1 className="kc-title">Propose a firing</h1>

      {activeKilns.length === 0 && <Alert kind="error">There are no active kilns. Ask an admin.</Alert>}
      {activeKilns.length > 1 && (
        <fieldset>
          <legend className="kc-field__label">Kiln</legend>
          <div className="kc-keys kc-keys--fill">
            {activeKilns.map((k) => (
              <button
                key={k.id}
                type="button"
                aria-pressed={kilnId === k.id}
                onClick={() => setKilnId(k.id)}
                className="kc-key"
              >
                {k.name}
              </button>
            ))}
          </div>
        </fieldset>
      )}

      <Field
        label="Firing profile"
        hint={
          <span className="kc-caption kc-caption--body block">
            Presets are set up per kiln. Choose Other for something new.
          </span>
        }
      >
        <span className="kc-select-wrap">
          <select value={profileId} onChange={(e) => setProfileId(e.target.value)} className="kc-select">
            {kilnProfiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
            <option value="__other">Other / custom…</option>
          </select>
        </span>
      </Field>

      {other ? (
        <section className="space-y-4">
          <Field label="Firing name">
            <input
              required
              maxLength={120}
              value={customName}
              onChange={(e) => setCustomName(e.target.value)}
              placeholder="Cone 06 Bisque — fast"
              className={inputClass}
            />
          </Field>
          <fieldset>
            <legend className="kc-field__label">Type</legend>
            <div className="kc-keys kc-keys--fill">
              {options.firing_types.map((t) => (
                <button
                  key={t}
                  type="button"
                  aria-pressed={customType === t}
                  onClick={() => setCustomType(t)}
                  className="kc-key capitalize"
                >
                  {t}
                </button>
              ))}
            </div>
          </fieldset>
          <ConeSelect value={customCone} onChange={setCustomCone} cones={options.cones} />
          <div className="grid grid-cols-2 gap-3">
            <Field label="Firing time (hours)">
              <input
                required
                type="number"
                inputMode="decimal"
                min={0.01}
                max={168}
                step={0.25}
                value={customFiringHours}
                onChange={(e) => setCustomFiringHours(e.target.value)}
                className={inputClass}
              />
            </Field>
            <Field label="Cooldown (hours)">
              <input
                required
                type="number"
                inputMode="decimal"
                min={0}
                max={168}
                step={0.25}
                value={customCooldownHours}
                onChange={(e) => setCustomCooldownHours(e.target.value)}
                className={inputClass}
              />
            </Field>
          </div>
          {canSaveProfile && (
            <label className="kc-check">
              <input type="checkbox" checked={saveProfile} onChange={(e) => setSaveProfile(e.target.checked)} className="kc-checkbox" />
              Save this as a preset for this kiln
            </label>
          )}
        </section>
      ) : (
        <Alert kind="info">
          {selected?.name} · <span className="capitalize">{selected?.firing_type}</span> cone {selected?.cone} · about{' '}
          {Math.round((firingMinutes / 60) * 10) / 10}h firing + {Math.round((cooldownMinutes / 60) * 10) / 10}h cooldown
        </Alert>
      )}

      <Field
        label="Day"
        hint={
          <span className="kc-caption kc-caption--body block">
            Booked for a day; the kiln runs sometime that day. Requests close {options.request_cutoff_hours}h before.
            {blackouts.length > 0 && (
              <>
                {' '}
                Unavailable:{' '}
                {blackouts
                  .map((b) => (b.start_date === b.end_date ? b.start_date : `${b.start_date}–${b.end_date}`))
                  .join(', ')}
                .
              </>
            )}
          </span>
        }
      >
        <input
          type="date"
          required
          min={canBypass ? undefined : earliest}
          value={day}
          onChange={(e) => setDay(e.target.value)}
          className={inputClass}
        />
      </Field>

      {blocked && <Alert kind="warning">That day is unavailable for this kiln. Pick another day.</Alert>}

      <section className="space-y-4">
        <h2 className="kc-title kc-title--sm">Your work</h2>
        <Field label="What are you putting in?">
          <textarea
            required
            rows={2}
            maxLength={1000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="4 mugs, 2 medium bowls, 1 tall vase"
            className="kc-textarea"
          />
        </Field>
        <ShareInput value={share} onChange={setShare} kiln={activeKilns.find((k) => k.id === kilnId)} />
        <Field label="Clay body (optional)">
          <input value={clayBody} maxLength={120} onChange={(e) => setClayBody(e.target.value)} className={inputClass} />
        </Field>
      </section>

      <Field label="Notes (optional)">
        <textarea rows={3} maxLength={2000} value={notes} onChange={(e) => setNotes(e.target.value)} className="kc-textarea" />
      </Field>

      {err && <Alert kind="error">{err}</Alert>}

      <button type="submit" disabled={busy || blocked || activeKilns.length === 0} className="kc-key kc-key--block kc-key--fire">
        {busy ? 'Proposing…' : 'Propose firing'}
      </button>
    </form>
  )
}
