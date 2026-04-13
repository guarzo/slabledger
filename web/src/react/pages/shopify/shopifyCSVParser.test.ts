import { describe, it, expect } from 'vitest';
import { splitCSVLine, splitCSVRows, quoteCSVField } from './shopifyCSVParser';

describe('splitCSVLine', () => {
  const cases = [
    {
      name: 'simple fields',
      input: 'a,b,c',
      expected: ['a', 'b', 'c'],
    },
    {
      name: 'quoted field with comma',
      input: '"hello, world",b,c',
      expected: ['hello, world', 'b', 'c'],
    },
    {
      name: 'double-quoted quote inside field',
      input: '"he said ""hi""",b',
      expected: ['he said "hi"', 'b'],
    },
    {
      name: 'empty fields',
      input: 'a,,c',
      expected: ['a', '', 'c'],
    },
    {
      name: 'quoted empty field',
      input: '"",b',
      expected: ['', 'b'],
    },
    {
      name: 'single field',
      input: 'hello',
      expected: ['hello'],
    },
    {
      name: 'all empty',
      input: ',,,',
      expected: ['', '', '', ''],
    },
    {
      name: 'field with embedded newline in quotes',
      input: '"line1\nline2",b',
      expected: ['line1\nline2', 'b'],
    },
  ];

  for (const tc of cases) {
    it(tc.name, () => {
      expect(splitCSVLine(tc.input)).toEqual(tc.expected);
    });
  }
});

describe('splitCSVRows', () => {
  it('splits on LF', () => {
    expect(splitCSVRows('a,b\nc,d')).toEqual(['a,b', 'c,d']);
  });

  it('splits on CRLF', () => {
    expect(splitCSVRows('a,b\r\nc,d')).toEqual(['a,b', 'c,d']);
  });

  it('ignores newlines inside quoted fields', () => {
    const input = '"a\nb",c\nd,e';
    const rows = splitCSVRows(input);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toContain('"a\nb"');
  });

  it('skips empty rows', () => {
    expect(splitCSVRows('a\n\nb')).toEqual(['a', 'b']);
  });
});

describe('quoteCSVField', () => {
  it('wraps field with commas in quotes', () => {
    expect(quoteCSVField('hello, world')).toBe('"hello, world"');
  });

  it('escapes double quotes', () => {
    expect(quoteCSVField('say "hello"')).toBe('"say ""hello"""');
  });

  it('does not wrap plain field', () => {
    expect(quoteCSVField('hello')).toBe('hello');
  });

  it('wraps field with newline', () => {
    expect(quoteCSVField('line1\nline2')).toBe('"line1\nline2"');
  });
});
