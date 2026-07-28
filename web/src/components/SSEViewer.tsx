import React from 'react';
import { Typography } from 'antd';

interface SSEViewerProps {
  text: string;
}

// Simple rendering of streaming text with basic markdown-like formatting.
// Handles code blocks (```), inline code (`), and plain text.
export default function SSEViewer({ text }: SSEViewerProps) {
  if (!text) return null;

  const rendered = renderMarkdownLike(text);

  return (
    <Typography.Paragraph
      style={{
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        marginBottom: 0,
      }}
    >
      {rendered}
    </Typography.Paragraph>
  );
}

function renderMarkdownLike(text: string): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  const lines = text.split('\n');
  let inCodeBlock = false;
  let codeBlockLines: string[] = [];
  let codeBlockIndex = 0;
  let paragraphLines: string[] = [];

  function flushParagraph() {
    if (paragraphLines.length === 0) return;
    const content = paragraphLines.join('\n');
    nodes.push(
      <span key={`p-${nodes.length}`}>
        {renderInlineMarkup(content)}
        <br />
      </span>,
    );
    paragraphLines = [];
  }

  function flushCodeBlock() {
    if (codeBlockLines.length === 0) return;
    nodes.push(
      <pre
        key={`cb-${codeBlockIndex++}`}
        style={{
          background: '#1e1e1e',
          color: '#d4d4d4',
          padding: '12px 16px',
          borderRadius: 8,
          overflow: 'auto',
          fontSize: '13px',
          lineHeight: 1.5,
          fontFamily: "'Fira Code', 'Cascadia Code', 'Consolas', monospace",
        }}
      >
        <code>{codeBlockLines.join('\n')}</code>
      </pre>,
    );
    codeBlockLines = [];
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    if (line.trim().startsWith('```')) {
      if (inCodeBlock) {
        flushCodeBlock();
        inCodeBlock = false;
      } else {
        flushParagraph();
        inCodeBlock = true;
      }
      continue;
    }

    if (inCodeBlock) {
      codeBlockLines.push(line);
    } else if (line.trim() === '') {
      flushParagraph();
    } else {
      paragraphLines.push(line);
    }
  }

  // Flush any remaining
  if (inCodeBlock) {
    flushCodeBlock();
  } else {
    flushParagraph();
  }

  return nodes;
}

function renderInlineMarkup(content: string): React.ReactNode {
  // Split by inline code markers
  const parts = content.split(/(`[^`]+`)/g);
  return parts.map((part, i) => {
    if (part.startsWith('`') && part.endsWith('`')) {
      return (
        <code
          key={i}
          style={{
            background: '#f0f0f0',
            padding: '2px 6px',
            borderRadius: 4,
            fontSize: '0.9em',
            fontFamily: "'Fira Code', 'Cascadia Code', 'Consolas', monospace",
          }}
        >
          {part.slice(1, -1)}
        </code>
      );
    }
    // Bold: **text**
    const boldParts = part.split(/(\*\*[^*]+\*\*)/g);
    return boldParts.map((bp, j) => {
      if (bp.startsWith('**') && bp.endsWith('**')) {
        return <strong key={`${i}-${j}`}>{bp.slice(2, -2)}</strong>;
      }
      return bp;
    });
  });
}
