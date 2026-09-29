import { useId, useState, type DragEvent, type ReactNode } from 'react'

type Props = {
  accept: string
  // label names the hidden file input for screen readers.
  label: string
  onFile: (file: File) => void
  disabled?: boolean
  className?: string
  // children draw the zone; choose opens the system file dialog, for a button or a link inside it.
  children: (choose: () => void) => ReactNode
}

function carriesFiles(e: DragEvent): boolean {
  return Array.from(e.dataTransfer.types).includes('Files')
}

// FileDrop takes a file dropped on it or picked in the system dialog. It takes the first file of a drop.
export function FileDrop({ accept, label, onFile, disabled, className, children }: Props) {
  const inputId = useId()
  const [over, setOver] = useState(false)
  const choose = () => document.getElementById(inputId)?.click()
  const classes = ['file-drop', className, over ? 'is-over' : ''].filter(Boolean).join(' ')

  return (
    <div
      className={classes}
      onDragOver={(e) => {
        if (disabled || !carriesFiles(e)) {
          return
        }
        e.preventDefault()
        e.dataTransfer.dropEffect = 'copy'
        setOver(true)
      }}
      onDragLeave={(e) => {
        if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) {
          setOver(false)
        }
      }}
      onDrop={(e) => {
        if (disabled || !carriesFiles(e)) {
          return
        }
        e.preventDefault()
        setOver(false)
        const file = e.dataTransfer.files[0]
        if (file) {
          onFile(file)
        }
      }}
    >
      <input
        id={inputId}
        className="file-input"
        type="file"
        accept={accept}
        aria-label={label}
        tabIndex={-1}
        disabled={disabled}
        onChange={(e) => {
          const file = e.target.files?.[0]
          e.target.value = ''
          if (file) {
            onFile(file)
          }
        }}
      />
      {children(choose)}
    </div>
  )
}
