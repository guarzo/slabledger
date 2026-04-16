export interface ParsedSection {
  heading: string;
  body: string;
}

/**
 * Split a markdown document into (heading, body) segments keyed off H2 lines.
 * Content before the first H2 is returned under an empty heading key.
 *
 * The parser is tolerant: trailing whitespace in heading text is trimmed, and
 * trailing blank lines in body content are preserved so the downstream
 * markdown renderer can re-hydrate paragraph spacing correctly.
 */
export function splitByH2(markdown: string): ParsedSection[] {
  if (!markdown) return [];
  const lines = markdown.split('\n');
  const sections: ParsedSection[] = [];
  let currentHeading = '';
  let buffer: string[] = [];

  const flush = () => {
    const body = buffer.join('\n').trim();
    if (currentHeading || body) {
      sections.push({ heading: currentHeading, body });
    }
    buffer = [];
  };

  const h2Re = /^##\s+(.+?)\s*$/;
  for (const line of lines) {
    const match = h2Re.exec(line);
    if (match) {
      flush();
      currentHeading = match[1].trim();
      continue;
    }
    buffer.push(line);
  }
  flush();
  return sections;
}
