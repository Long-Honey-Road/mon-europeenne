import Markdown from 'react-markdown'
import type { Components, Options } from 'react-markdown'
import rehypeHighlight from 'rehype-highlight'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import bash from 'highlight.js/lib/languages/bash'
import go from 'highlight.js/lib/languages/go'
import json from 'highlight.js/lib/languages/json'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import yaml from 'highlight.js/lib/languages/yaml'
import 'katex/dist/katex.min.css'
import { remarkDisplayMath } from './displayMath'

// Raw HTML stays off (no rehype-raw): the model's text is never trusted as markup.
const remarkPlugins: Options['remarkPlugins'] = [remarkGfm, remarkMath, remarkDisplayMath]
const rehypePlugins: Options['rehypePlugins'] = [
  [rehypeKatex, { throwOnError: false }],
  [rehypeHighlight, { languages: { bash, go, json, sql, typescript, yaml } }],
]

const components: Components = {
  // Wide tables scroll inside the bubble instead of stretching the chat panel.
  table: ({ node: _node, ...props }) => (
    <div className="msg__table">
      <table {...props} />
    </div>
  ),
}

export function Answer({ text }: { text: string }) {
  return (
    <div className="msg__text">
      <Markdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={components}>
        {text}
      </Markdown>
    </div>
  )
}
