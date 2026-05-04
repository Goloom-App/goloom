import type { ReactNode } from 'react'

export function SettingsCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="subpanel">
      <h3>{title}</h3>
      {children}
    </section>
  )
}
