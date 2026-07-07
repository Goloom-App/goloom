# Self-hosted fonts

These woff2 files are served locally so the marketing site depends on no
third-party font CDN at runtime — fitting for a self-hosted product.

| Family | Use | License |
|---|---|---|
| Bricolage Grotesque | display / headings | SIL Open Font License 1.1 |
| IBM Plex Sans | body text | SIL Open Font License 1.1 |
| IBM Plex Mono | terminal / code | SIL Open Font License 1.1 |

Latin subset only. Sourced from the Fontsource distribution
(https://fontsource.org). Both families are OFL-licensed, which permits
bundling and redistribution. To refresh, re-download the matching
`latin-<weight>-normal.woff2` files and keep the `@font-face` weights in
`src/styles/fonts.css` in sync.
