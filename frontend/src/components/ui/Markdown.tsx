import { Fragment, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

/** Minimal, safe Markdown → React (no HTML injection): headings, lists, quotes, code, tables, rules, inline emphasis/links. */
export function Markdown({ text, className }: { text: string; className?: string }) {
  return <div className={cn('md-body', className)}>{renderBlocks(text)}</div>
}

const INLINE = /(\*\*[^*]+\*\*|__[^_]+__|`[^`]+`|\*[^*\s][^*]*\*|_[^_\s][^_]*_|\[[^\]]+\]\([^)\s]+\))/g

function renderInline(s: string, keyBase = 'i'): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let k = 0
  for (const m of s.matchAll(INLINE)) {
    const t = m[0]
    if (m.index > last) out.push(s.slice(last, m.index))
    const key = `${keyBase}-${k++}`
    if (t.startsWith('**') || t.startsWith('__')) out.push(<strong key={key}>{renderInline(t.slice(2, -2), key)}</strong>)
    else if (t.startsWith('`')) out.push(<code key={key}>{t.slice(1, -1)}</code>)
    else if (t.startsWith('[')) {
      const lm = /^\[([^\]]+)\]\(([^)\s]+)\)$/.exec(t)
      const href = lm?.[2] ?? ''
      out.push(
        /^https?:\/\//i.test(href) ? (
          <a key={key} href={href} target="_blank" rel="noopener noreferrer" className="text-accent underline underline-offset-2">
            {lm?.[1]}
          </a>
        ) : (
          lm?.[1]
        ),
      )
    } else out.push(<em key={key}>{renderInline(t.slice(1, -1), key)}</em>)
    last = m.index + t.length
  }
  if (last < s.length) out.push(s.slice(last))
  return out
}

const cells = (row: string) =>
  row
    .trim()
    .replace(/^\||\|$/g, '')
    .split('|')
    .map((c) => c.trim())

function renderBlocks(text: string): ReactNode[] {
  const lines = text.replace(/\r\n?/g, '\n').split('\n')
  const out: ReactNode[] = []
  let i = 0
  let k = 0
  while (i < lines.length) {
    const line = lines[i]
    const key = `b${k++}`
    if (!line.trim()) {
      i++
      continue
    }
    if (line.startsWith('```')) {
      const buf: string[] = []
      i++
      while (i < lines.length && !lines[i].startsWith('```')) buf.push(lines[i++])
      i++
      out.push(
        <pre key={key}>
          <code>{buf.join('\n')}</code>
        </pre>,
      )
      continue
    }
    const h = /^(#{1,6})\s+(.*)$/.exec(line)
    if (h) {
      const level = Math.min(4, Math.max(2, h[1].length + 1))
      const Tag = `h${level}` as 'h2' | 'h3' | 'h4'
      out.push(<Tag key={key}>{renderInline(h[2], key)}</Tag>)
      i++
      continue
    }
    if (/^\s*([-*_])\s*\1\s*\1[\s\-*_]*$/.test(line)) {
      out.push(<hr key={key} />)
      i++
      continue
    }
    if (line.trim().startsWith('|') && i + 1 < lines.length && /^\s*\|?\s*:?-{2,}/.test(lines[i + 1])) {
      const head = cells(line)
      i += 2
      const rows: string[][] = []
      while (i < lines.length && lines[i].trim().startsWith('|')) rows.push(cells(lines[i++]))
      out.push(
        <table key={key}>
          <thead>
            <tr>
              {head.map((c, j) => (
                <th key={j}>{renderInline(c, `${key}h${j}`)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r, ri) => (
              <tr key={ri}>
                {r.map((c, j) => (
                  <td key={j}>{renderInline(c, `${key}r${ri}c${j}`)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>,
      )
      continue
    }
    if (line.startsWith('>')) {
      const buf: string[] = []
      while (i < lines.length && lines[i].startsWith('>')) buf.push(lines[i++].replace(/^>\s?/, ''))
      out.push(<blockquote key={key}>{renderBlocks(buf.join('\n'))}</blockquote>)
      continue
    }
    const ul = /^\s*[-*+]\s+/
    const ol = /^\s*\d+[.)]\s+/
    if (ul.test(line) || ol.test(line)) {
      const ordered = ol.test(line)
      const re = ordered ? ol : ul
      const items: string[] = []
      while (i < lines.length && re.test(lines[i])) items.push(lines[i++].replace(re, ''))
      const Tag = ordered ? 'ol' : 'ul'
      out.push(
        <Tag key={key}>
          {items.map((it, j) => (
            <li key={j}>{renderInline(it, `${key}l${j}`)}</li>
          ))}
        </Tag>,
      )
      continue
    }
    const buf: string[] = []
    while (i < lines.length && lines[i].trim() && !/^(#{1,6}\s|```|>|\s*[-*+]\s|\s*\d+[.)]\s|\s*\|)/.test(lines[i])) buf.push(lines[i++])
    if (!buf.length) buf.push(lines[i++])
    out.push(
      <p key={key}>
        {buf.map((b, j) => (
          <Fragment key={j}>
            {j > 0 && <br />}
            {renderInline(b, `${key}p${j}`)}
          </Fragment>
        ))}
      </p>,
    )
  }
  return out
}
