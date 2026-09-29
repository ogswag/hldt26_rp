import type { CanvasPalette } from '../ui/theme'

const stripPadding = 12
const lineGap = 6
const titleSize = 16
const textSize = 13

// composeFrame puts a caption strip under a picture of the stage: the stage is drawn on a canvas, but the variant,
// the clock and the counters live in the page, so a saved frame would not name what it shows.
export function composeFrame(stage: HTMLCanvasElement, lines: string[], pal: CanvasPalette, ratio: number): HTMLCanvasElement {
  const pad = stripPadding * ratio
  const gap = lineGap * ratio
  const stripHeight = pad * 2 + lines.length * (textSize * ratio) + (lines.length - 1) * gap + (titleSize - textSize) * ratio
  const out = document.createElement('canvas')
  out.width = stage.width
  out.height = stage.height + Math.ceil(stripHeight)
  const ctx = out.getContext('2d')
  if (!ctx) {
    throw new Error('frame.compose: the canvas has no 2d context')
  }
  ctx.fillStyle = pal.paper
  ctx.fillRect(0, 0, out.width, out.height)
  ctx.drawImage(stage, 0, 0)
  ctx.fillStyle = pal.line
  ctx.fillRect(0, stage.height, out.width, Math.max(1, ratio))
  ctx.textBaseline = 'top'
  let y = stage.height + pad
  lines.forEach((line, i) => {
    const size = (i === 0 ? titleSize : textSize) * ratio
    ctx.font = `${i === 0 ? 600 : 400} ${size}px ${pal.font}`
    ctx.fillStyle = i === 0 ? pal.ink : pal.inkMuted
    ctx.fillText(line, pad, y)
    y += size + gap
  })
  return out
}

export function canvasBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('frame.blob: the canvas gave no image'))), 'image/png')
  })
}
