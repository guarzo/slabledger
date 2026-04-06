import type { CSVFormat, CSVRow, ParsedCSV } from './shopifyTypes';

/** Split a single CSV line respecting quoted fields (RFC 4180). */
export function splitCSVLine(line: string): string[] {
  const fields: string[] = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (inQuotes) {
      if (ch === '"') {
        if (i + 1 < line.length && line[i + 1] === '"') {
          current += '"';
          i++; // skip escaped quote
        } else {
          inQuotes = false;
        }
      } else {
        current += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
    } else if (ch === ',') {
      fields.push(current);
      current = '';
    } else {
      current += ch;
    }
  }
  fields.push(current);
  return fields;
}

/** Split CSV text into rows, respecting newlines inside quoted fields (RFC 4180). */
export function splitCSVRows(text: string): string[] {
  const rows: string[] = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (inQuotes) {
      if (ch === '"') {
        if (i + 1 < text.length && text[i + 1] === '"') {
          current += '""';
          i++;
        } else {
          inQuotes = false;
          current += ch;
        }
      } else {
        current += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
      current += ch;
    } else if (ch === '\r') {
      if (i + 1 < text.length && text[i + 1] === '\n') {
        i++; // skip \n after \r
      }
      if (current.trim()) rows.push(current);
      current = '';
    } else if (ch === '\n') {
      if (current.trim()) rows.push(current);
      current = '';
    } else {
      current += ch;
    }
  }
  if (current.trim()) rows.push(current);
  return rows;
}

/** Quote a field for CSV output if it contains commas, quotes, or newlines. */
export function quoteCSVField(field: string): string {
  if (field.includes(',') || field.includes('"') || field.includes('\n') || field.includes('\r')) {
    return '"' + field.replace(/"/g, '""') + '"';
  }
  return field;
}

/** Find a column index matching any of the candidate names (case-insensitive, trimmed).
 *  Priority: exact match > word-boundary match > substring match. */
function findColumn(headers: string[], ...candidates: string[]): number {
  const lower = headers.map(h => h.trim().toLowerCase());
  // Exact match first
  for (const c of candidates) {
    const idx = lower.indexOf(c);
    if (idx >= 0) return idx;
  }
  // Word-boundary match
  for (const c of candidates) {
    const re = new RegExp(`\\b${c.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`);
    const idx = lower.findIndex(h => re.test(h));
    if (idx >= 0) return idx;
  }
  // Substring fallback
  for (const c of candidates) {
    const idx = lower.findIndex(h => h.includes(c));
    if (idx >= 0) return idx;
  }
  return -1;
}

export function detectAndParseCSV(text: string): ParsedCSV {
  // Strip BOM
  const clean = text.replace(/^\uFEFF/, '');
  const rawLines = splitCSVRows(clean);
  if (rawLines.length < 2) {
    return { format: 'shopify', headers: [], prefixLines: [], items: [], certIdx: -1, priceIdx: -1 };
  }

  // Detect eBay format: first line starts with info/version marker
  const isEbay = rawLines[0].startsWith('Info,');
  const prefixLines = isEbay ? [rawLines[0]] : [];
  const headerLine = isEbay ? rawLines[1] : rawLines[0];
  const dataLines = isEbay ? rawLines.slice(2) : rawLines.slice(1);
  const format: CSVFormat = isEbay ? 'ebay' : 'shopify';

  const headers = splitCSVLine(headerLine);
  const rows = dataLines.map(line => splitCSVLine(line));

  // Find cert column: eBay uses "certification number", Shopify uses "cert number" or "cert"
  const certIdx = findColumn(headers, 'certification number', 'cert number', 'cert');
  // Find price column: eBay uses "*startprice", Shopify uses "price"
  const priceIdx = findColumn(headers, '*startprice', 'price');
  // Find title column
  const titleIdx = findColumn(headers, '*title', 'title');
  // Find SKU column
  const skuIdx = findColumn(headers, 'customlabel', 'sku');

  const items: CSVRow[] = [];
  for (const row of rows) {
    let certNumber = certIdx >= 0 ? row[certIdx]?.trim() : '';
    let grader = '';

    if (!certNumber && skuIdx >= 0) {
      const sku = row[skuIdx]?.trim() || '';
      const psaMatch = sku.match(/^PSA-(\d+)$/i);
      if (psaMatch) {
        certNumber = psaMatch[1];
        grader = 'PSA';
      }
    }

    items.push({
      raw: row,
      certNumber,
      grader,
      price: priceIdx >= 0 ? row[priceIdx]?.trim() || '' : '',
      title: titleIdx >= 0 ? row[titleIdx]?.trim() || '' : '',
    });
  }

  return { format, headers, prefixLines, items, certIdx, priceIdx };
}
