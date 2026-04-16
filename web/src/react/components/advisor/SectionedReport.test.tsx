import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import SectionedReport from './SectionedReport';
import { splitByH2 } from './splitByH2';

describe('splitByH2', () => {
  it('splits markdown on H2 headings', () => {
    const md = '## Alpha\nalpha body\n\n## Beta\nbeta body';
    const sections = splitByH2(md);
    expect(sections).toEqual([
      { heading: 'Alpha', body: 'alpha body' },
      { heading: 'Beta', body: 'beta body' },
    ]);
  });

  it('retains content before the first H2 under an empty heading', () => {
    const md = 'preamble text\n\n## First\nbody';
    const sections = splitByH2(md);
    expect(sections[0]).toEqual({ heading: '', body: 'preamble text' });
    expect(sections[1]).toEqual({ heading: 'First', body: 'body' });
  });

  it('trims trailing whitespace in headings', () => {
    const md = '##   Spaced Title   \nbody';
    expect(splitByH2(md)).toEqual([{ heading: 'Spaced Title', body: 'body' }]);
  });

  it('returns empty for empty input', () => {
    expect(splitByH2('')).toEqual([]);
  });
});

describe('SectionedReport', () => {
  const schema = [
    { heading: 'Top Actions', icon: '🎯' },
    { heading: 'Portfolio Performance', icon: '📊' },
  ];

  it('renders schema sections in schema order, regardless of LLM ordering', () => {
    const markdown = '## Portfolio Performance\nperformance body\n\n## Top Actions\naction body';
    const { container } = render(<SectionedReport markdown={markdown} schema={schema} cacheKey="test-1" />);
    const headings = Array.from(container.querySelectorAll('h3')).map(h => h.textContent?.trim());
    expect(headings.slice(0, 2)).toEqual([
      expect.stringContaining('Top Actions'),
      expect.stringContaining('Portfolio Performance'),
    ]);
  });

  it('marks missing schema sections as not-generated placeholders', () => {
    const markdown = '## Top Actions\naction body';
    render(<SectionedReport markdown={markdown} schema={schema} cacheKey="test-2" />);
    expect(screen.getByText(/not generated/i)).toBeInTheDocument();
    expect(screen.getByText(/did not produce this section/i)).toBeInTheDocument();
  });

  it('renders extra (off-schema) sections under Additional Notes so nothing is hidden', () => {
    const markdown = '## Top Actions\naction body\n\n## Surprise Section\nextra data';
    render(<SectionedReport markdown={markdown} schema={schema} cacheKey="test-3" />);
    expect(screen.getByText(/additional notes/i)).toBeInTheDocument();
    expect(screen.getByText('Surprise Section')).toBeInTheDocument();
  });
});
