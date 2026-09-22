import { useEffect } from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Underline from '@tiptap/extension-underline'
import Placeholder from '@tiptap/extension-placeholder'
import {
  Bold,
  Code,
  Italic,
  Link as LinkIcon,
  List,
  ListOrdered,
  Quote,
  Underline as UnderlineIcon,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

interface RichTextEditorProps {
  html: string
  onChange: (value: { html: string; text: string }) => void
  placeholder?: string
}

/**
 * The compose body editor.
 *
 * Both representations are reported on every change: the HTML is what goes out
 * and what the reader sees, while the plain text is the fallback part of the
 * message and what search will index.
 */
export function RichTextEditor({ html, onChange, placeholder }: RichTextEditorProps) {
  const editor = useEditor({
    extensions: [
      StarterKit.configure({ heading: { levels: [1, 2, 3] } }),
      Underline,
      Link.configure({
        openOnClick: false,
        autolink: true,
        // Composed links are still links in somebody else's inbox, so they
        // carry the usual protections rather than relying on the reader's client.
        HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
      }),
      Placeholder.configure({ placeholder: placeholder ?? 'Write your message…' }),
    ],
    content: html,
    editorProps: {
      attributes: {
        class:
          'prose prose-sm max-w-none min-h-48 px-3 py-2 focus:outline-none [&_p]:my-1 [&_ul]:my-1 [&_ol]:my-1',
        'aria-label': 'Message body',
      },
    },
    onUpdate: ({ editor: instance }) => {
      onChange({ html: instance.getHTML(), text: instance.getText() })
    },
  })

  // Loading an existing draft replaces the content once; typing must not be
  // interrupted by echoing our own updates back in.
  useEffect(() => {
    if (!editor) return
    if (html !== editor.getHTML()) {
      editor.commands.setContent(html, { emitUpdate: false })
    }
    // Only react to a new draft being loaded, not to every keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor])

  if (!editor) return null

  const toggles = [
    { label: 'Bold', icon: Bold, isActive: () => editor.isActive('bold'), run: () => editor.chain().focus().toggleBold().run() },
    { label: 'Italic', icon: Italic, isActive: () => editor.isActive('italic'), run: () => editor.chain().focus().toggleItalic().run() },
    { label: 'Underline', icon: UnderlineIcon, isActive: () => editor.isActive('underline'), run: () => editor.chain().focus().toggleUnderline().run() },
    { label: 'Bulleted list', icon: List, isActive: () => editor.isActive('bulletList'), run: () => editor.chain().focus().toggleBulletList().run() },
    { label: 'Numbered list', icon: ListOrdered, isActive: () => editor.isActive('orderedList'), run: () => editor.chain().focus().toggleOrderedList().run() },
    { label: 'Quote', icon: Quote, isActive: () => editor.isActive('blockquote'), run: () => editor.chain().focus().toggleBlockquote().run() },
    { label: 'Code', icon: Code, isActive: () => editor.isActive('code'), run: () => editor.chain().focus().toggleCode().run() },
  ]

  const setLink = () => {
    const previous = editor.getAttributes('link').href as string | undefined
    const href = window.prompt('Link URL', previous ?? 'https://')
    if (href === null) return

    if (href === '') {
      editor.chain().focus().extendMarkRange('link').unsetLink().run()
      return
    }
    // Only http(s) — a javascript: or data: URL in a message is an attack.
    if (!/^https?:\/\//i.test(href)) {
      window.alert('Links must start with http:// or https://')
      return
    }
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run()
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-center gap-0.5 border-y px-2 py-1">
        {toggles.map((toggle) => (
          <Button
            key={toggle.label}
            type="button"
            variant="ghost"
            size="icon"
            aria-label={toggle.label}
            aria-pressed={toggle.isActive()}
            className={cn('size-8', toggle.isActive() && 'bg-accent')}
            onClick={toggle.run}
          >
            <toggle.icon className="size-4" />
          </Button>
        ))}

        <Separator orientation="vertical" className="mx-1 h-5" />

        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Link"
          aria-pressed={editor.isActive('link')}
          className={cn('size-8', editor.isActive('link') && 'bg-accent')}
          onClick={setLink}
        >
          <LinkIcon className="size-4" />
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        <EditorContent editor={editor} />
      </div>
    </div>
  )
}
