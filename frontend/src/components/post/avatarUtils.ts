export function avatarBackground(color: string) {
  return `linear-gradient(135deg, ${color}, color-mix(in srgb, ${color} 40%, white))`
}
