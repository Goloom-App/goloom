/** Stacked-square mark matching `goloom-logo` styles in `index.css` (sidebar, auth, favicon.svg). */
export function GoloomLogo({
  className = '',
  size = 'md',
}: {
  className?: string
  /** `md` = 40px (sidebar); `lg` = 48px (login hero). */
  size?: 'md' | 'lg'
}) {
  return (
    <div
      className={`goloom-logo goloom-logo--${size} ${className}`.trim()}
      title="goloom"
      aria-hidden="true"
    >
      <span className="goloom-logo__layer goloom-logo__layer--a" />
      <span className="goloom-logo__layer goloom-logo__layer--b" />
      <span className="goloom-logo__layer goloom-logo__layer--c" />
    </div>
  )
}
