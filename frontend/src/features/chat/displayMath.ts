// Structural mdast types: only the fields this plugin reads.
type Node = { type: string; value?: string; children?: Node[]; position?: { start: { offset?: number } }; data?: Data }
type Data = { hProperties?: { className?: string[] } }

const blank = (n: Node) => n.type === 'text' && !n.value?.trim()

/**
 * Models write display math on one line (`$$\frac{a}{b}$$`), which remark-math parses as inline math.
 * A paragraph holding only such math is shown as a display formula.
 */
export function remarkDisplayMath() {
  return (tree: Node, file: { value: unknown }) => {
    const source = String(file.value)
    const walk = (node: Node) => {
      const kids = node.children ?? []
      if (node.type === 'paragraph') {
        const content = kids.filter((k) => !blank(k))
        const [math] = content
        const start = math?.position?.start.offset
        if (content.length === 1 && math?.type === 'inlineMath' && start !== undefined && source.startsWith('$$', start)) {
          math.data = { ...math.data, hProperties: { className: ['language-math', 'math-display'] } }
        }
        return
      }
      kids.forEach(walk)
    }
    walk(tree)
  }
}
